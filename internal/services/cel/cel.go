// Package cel evaluates ABAC schema conditions at the edge. It runs the backend's already
// type-checked CEL program (shipped as exprpb.CheckedExpr) over a direct cel-go dependency, reusing
// the backend's custom CEL types (the ipaddress type/functions). The edge never parses or type-checks
// the DSL — parity with the backend rests only on the cel-go version + the shared custom functions.
package cel

import (
	"context"
	"fmt"
	"time"

	"github.com/descope/backend/authzservice/pkg/authzservice/cel/descopecel"
	cctx "github.com/descope/backend/common/pkg/common/context"
	"github.com/descope/go-sdk/descope"
	"github.com/google/cel-go/cel"
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
	"google.golang.org/protobuf/proto"
)

// CompiledCondition is a compiled CEL condition together with its typed parameters,
// ready to evaluate against a request context.
type CompiledCondition struct {
	name    string
	program cel.Program
	params  []*descope.FGAConditionParam
}

// Compile builds an executable program from the backend's type-checked CEL program (CheckedExpr). The
// edge does not parse or type-check the expression — it runs the backend's checked AST in an env with the
// shared custom functions/types (EnvOptions).
func Compile(c *descope.FGACondition) (*CompiledCondition, error) {
	if len(c.CheckedExpr) == 0 {
		return nil, fmt.Errorf("condition %q has no checked expression", c.Name)
	}
	env, err := cel.NewEnv(descopecel.EnvOptions()...)
	if err != nil {
		return nil, err
	}
	var checked exprpb.CheckedExpr
	if err := proto.Unmarshal(c.CheckedExpr, &checked); err != nil {
		return nil, err
	}
	program, err := env.Program(cel.CheckedExprToAst(&checked))
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
			cctx.Logger(ctx).Warn().Str("condition", cc.name).Str("param", p.Name).Msg("Condition param missing from request context, deferring to backend")
			return false, false
		}
		v, coerced := coerce(p.Type, raw)
		if !coerced {
			cctx.Logger(ctx).Warn().Str("condition", cc.name).Str("param", p.Name).Msg("Condition param could not be coerced, deferring to backend")
			return false, false
		}
		vars[p.Name] = v
	}
	evalCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, _, err := cc.program.ContextEval(evalCtx, vars)
	if err != nil {
		cctx.Logger(ctx).Warn().Str("condition", cc.name).Err(err).Msg("Condition evaluation failed, deferring to backend")
		return false, false
	}
	b, isBool := out.Value().(bool)
	if !isBool {
		cctx.Logger(ctx).Warn().Str("condition", cc.name).Msg("Condition did not evaluate to a bool, deferring to backend")
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
		switch n := raw.(type) {
		case float64:
			return int64(n), true
		case int64:
			return n, true
		case int:
			return int64(n), true
		default:
			return nil, false
		}
	case "uint":
		switch n := raw.(type) {
		case float64:
			return uint64(n), true
		case uint64:
			return n, true
		default:
			return nil, false
		}
	case descopecel.IPAddressTypeName:
		s, isStr := raw.(string)
		if !isStr {
			return nil, false
		}
		v, err := descopecel.NewIPAddressVal(s)
		if err != nil {
			return nil, false
		}
		return v, true
	default:
		return raw, true
	}
}
