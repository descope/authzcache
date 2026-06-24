// Package cel compiles and evaluates ABAC schema conditions at the edge. It reuses the
// backend's custom CEL types (the ipaddress type/functions and the DSL→cel type mapping)
// over a direct cel-go dependency, so the edge re-evaluates the exact expressions the
// backend returns for each condition — without importing the backend CEL runtime.
package cel

import (
	"context"
	"time"

	celtypes "github.com/descope/backend/authzservice/pkg/authzservice/cel/types"
	cctx "github.com/descope/backend/common/pkg/common/context"
	"github.com/descope/go-sdk/descope"
	"github.com/google/cel-go/cel"
)

// CompiledCondition is a compiled CEL condition together with its typed parameters,
// ready to evaluate against a request context.
type CompiledCondition struct {
	name    string
	program cel.Program
	params  []*descope.FGAConditionParam
}

// Compile builds a CEL program for a single schema condition using the shared custom env
// (ipaddress type/functions) plus a typed variable per declared parameter.
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

// Eval reports whether the condition evaluates to true for the given request context.
// ok is false when the condition cannot be evaluated at the edge — a missing or wrong-typed
// parameter, a non-bool result, an evaluation error, or a timeout — in which case the caller
// must defer the decision to the backend rather than trust a stale cached grant.
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

// coerce maps a JSON-decoded request-context value to the Go type cel-go expects for the
// declared DSL parameter type. JSON numbers decode to float64, so integer params need a
// conversion, and the custom ipaddress type is built from its string form. Other types pass
// through; a genuine mismatch surfaces as an eval error and defers to the backend.
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
