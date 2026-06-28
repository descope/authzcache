package cel_test

import (
	"context"
	"testing"
	"time"

	edgecel "github.com/descope/authzcache/internal/services/cel"
	celtypes "github.com/descope/backend/authzservice/pkg/authzservice/cel/descopecel"
	"github.com/descope/go-sdk/descope"
	"github.com/google/cel-go/cel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

const evalTimeout = time.Second

// mustCheckedCond builds an FGACondition carrying a serialized type-checked CEL program, exactly the way
// the backend produces it (compile in an env with the shared custom types + per-param vars, then
// AstToCheckedExpr). The edge runs this checked program rather than re-parsing the expression.
func mustCheckedCond(t *testing.T, name string, params []*descope.FGAConditionParam, expr string) *descope.FGACondition {
	t.Helper()
	opts := celtypes.EnvOptions()
	for _, p := range params {
		ct, err := celtypes.DSLTypeToCEL(p.Type)
		require.NoError(t, err)
		opts = append(opts, cel.Variable(p.Name, ct))
	}
	env, err := cel.NewEnv(opts...)
	require.NoError(t, err)
	ast, iss := env.Compile(expr)
	require.NoError(t, iss.Err())
	checked, err := cel.AstToCheckedExpr(ast)
	require.NoError(t, err)
	b, err := proto.Marshal(checked)
	require.NoError(t, err)
	return &descope.FGACondition{Name: name, Params: params, CheckedExpr: b}
}

func TestCompileAndEval(t *testing.T) {
	cond := mustCheckedCond(t, "isAdmin", []*descope.FGAConditionParam{{Name: "role", Type: "string"}}, `role == "admin"`)
	compiled, err := edgecel.Compile(cond)
	require.NoError(t, err)

	t.Run("condition true", func(t *testing.T) {
		pass, ok := compiled.Eval(context.Background(), map[string]any{"role": "admin"}, evalTimeout)
		require.True(t, ok)
		assert.True(t, pass)
	})
	t.Run("condition false", func(t *testing.T) {
		pass, ok := compiled.Eval(context.Background(), map[string]any{"role": "user"}, evalTimeout)
		require.True(t, ok)
		assert.False(t, pass)
	})
	t.Run("missing param defers to backend", func(t *testing.T) {
		_, ok := compiled.Eval(context.Background(), map[string]any{}, evalTimeout)
		assert.False(t, ok)
	})
	t.Run("wrong-typed param never yields a positive grant", func(t *testing.T) {
		// a wrong-typed value must not be served as allowed — either it can't evaluate (ok=false)
		// or the cross-type comparison is false; both keep the result out of the cache.
		pass, ok := compiled.Eval(context.Background(), map[string]any{"role": 42}, evalTimeout)
		assert.False(t, ok && pass)
	})
}

func TestEvalIntCoercion(t *testing.T) {
	// JSON numbers decode to float64; an int param must be coerced to int64 to match cel-go.
	cond := mustCheckedCond(t, "highClearance", []*descope.FGAConditionParam{{Name: "level", Type: "int"}}, `level >= 5`)
	compiled, err := edgecel.Compile(cond)
	require.NoError(t, err)

	pass, ok := compiled.Eval(context.Background(), map[string]any{"level": float64(7)}, evalTimeout)
	require.True(t, ok)
	assert.True(t, pass)

	pass, ok = compiled.Eval(context.Background(), map[string]any{"level": float64(3)}, evalTimeout)
	require.True(t, ok)
	assert.False(t, pass)
}

func TestEvalIPAddress(t *testing.T) {
	// custom ipaddress type + in_cidr function, shared with the backend via pkg.
	cond := mustCheckedCond(t, "inOfficeRange", []*descope.FGAConditionParam{{Name: "ip", Type: "ipaddress"}}, `ip.in_cidr("10.0.0.0/8")`)
	compiled, err := edgecel.Compile(cond)
	require.NoError(t, err)

	pass, ok := compiled.Eval(context.Background(), map[string]any{"ip": "10.1.2.3"}, evalTimeout)
	require.True(t, ok)
	assert.True(t, pass)

	pass, ok = compiled.Eval(context.Background(), map[string]any{"ip": "192.168.0.1"}, evalTimeout)
	require.True(t, ok)
	assert.False(t, pass)

	_, ok = compiled.Eval(context.Background(), map[string]any{"ip": "not-an-ip"}, evalTimeout)
	assert.False(t, ok, "an unparseable ip must defer to the backend")
}

func TestCompileRejectsMissingCheckedExpr(t *testing.T) {
	_, err := edgecel.Compile(&descope.FGACondition{Name: "empty"})
	require.Error(t, err, "a condition with no checked expression must not compile")
}

func TestCompileUnknownParamType(t *testing.T) {
	_, err := edgecel.Compile(&descope.FGACondition{
		Name:        "weird",
		Params:      []*descope.FGAConditionParam{{Name: "x", Type: "nonexistent"}},
		CheckedExpr: []byte("x"), // non-empty so Compile reaches the param-type mapping
	})
	require.Error(t, err)
}
