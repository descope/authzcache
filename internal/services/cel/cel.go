// Package cel compiles and evaluates ABAC schema conditions at the edge, reusing the backend's custom CEL types (pkg) over a direct cel-go dependency instead of the internal CEL runtime.
package cel

import (
	"context"
	"time"

	celtypes "github.com/descope/backend/authzservice/pkg/authzservice/cel/types"
	cctx "github.com/descope/backend/common/pkg/common/context"
	"github.com/descope/go-sdk/descope"
	"github.com/google/cel-go/cel"
)

// CompiledCondition is a compiled CEL condition plus its typed parameters, ready to evaluate against a request context.
type CompiledCondition struct {
	name    string
	program cel.Program
	params  []*descope.FGAConditionParam
}

// Compile builds a CEL program for one schema condition using the shared custom env plus a typed variable per parameter.
func Compile(c *descope.FGACondition) (*CompiledCondition, error) {
	opts := celtypes.EnvOptions()
	for _, p := range c.Params {
		t, err := celtypes.DSLTypeToCEL(p.Type)
		if err != nil {
			return nil, err
		}
		opts = append(opts, cel.Variable(p.Name, t))
	}
	env, err := cel.NewEnv(opts...)
	if err != nil {
		return nil, err
	}
	ast, iss := env.Compile(c.Expression)
	if iss.Err() != nil {
		return nil, iss.Err()
	}
	program, err := env.Program(ast)
	if err != nil {
		return nil, err
	}
	return &CompiledCondition{name: c.Name, program: program, params: c.Params}, nil
}

// Eval reports whether the condition is true for the request context; ok is false when it can't be evaluated at the edge (missing/wrong-typed param, non-bool, error, timeout) and the caller must defer to the backend.
func (cc *CompiledCondition) Eval(ctx context.Context, requestContext map[string]any, timeout time.Duration) (pass bool, ok bool) {
	vars := make(map[string]any, len(cc.params))
	for _, p := range cc.params {
		raw, present := requestContext[p.Name]
		if !present {
			cctx.Logger(ctx).Debug().Str("condition", cc.name).Str("param", p.Name).Msg("Condition param missing from request context, deferring to backend")
			return false, false
		}
		v, coerced := coerce(p.Type, raw)
		if !coerced {
			cctx.Logger(ctx).Debug().Str("condition", cc.name).Str("param", p.Name).Msg("Condition param could not be coerced, deferring to backend")
			return false, false
		}
		vars[p.Name] = v
	}
	evalCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, _, err := cc.program.ContextEval(evalCtx, vars)
	if err != nil {
		cctx.Logger(ctx).Debug().Str("condition", cc.name).Err(err).Msg("Condition evaluation failed, deferring to backend")
		return false, false
	}
	b, isBool := out.Value().(bool)
	if !isBool {
		cctx.Logger(ctx).Debug().Str("condition", cc.name).Msg("Condition did not evaluate to a bool, deferring to backend")
		return false, false
	}
	return b, true
}

// coerce maps a JSON-decoded value to the Go type cel-go expects for the DSL param type (float64->int64 for ints, string->ipaddress); other types pass through and a mismatch defers to the backend.
func coerce(dslType string, raw any) (any, bool) {
	switch dslType {
	case "int":
		// JSON numbers decode to float64; cel-go's int type needs int64
		if f, ok := raw.(float64); ok {
			return int64(f), true
		}
		return nil, false
	case "uint":
		if f, ok := raw.(float64); ok {
			return uint64(f), true
		}
		return nil, false
	case celtypes.IPAddressTypeName:
		s, isStr := raw.(string)
		if !isStr {
			return nil, false
		}
		v, err := celtypes.NewIPAddressVal(s)
		if err != nil {
			return nil, false
		}
		return v, true
	default:
		return raw, true
	}
}
