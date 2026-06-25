package cel_test

import (
	"context"
	"testing"
	"time"

	edgecel "github.com/descope/authzcache/internal/services/cel"
	"github.com/descope/go-sdk/descope"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const evalTimeout = time.Second

func TestCompileAndEval(t *testing.T) {
	cond := &descope.FGACondition{
		Name:       "isAdmin",
		Params:     []*descope.FGAConditionParam{{Name: "role", Type: "string"}},
		Expression: `role == "admin"`,
	}
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
		// wrong-typed value must not be served as allowed (ok=false, or cross-type compare is false)
		pass, ok := compiled.Eval(context.Background(), map[string]any{"role": 42}, evalTimeout)
		assert.False(t, ok && pass)
	})
}

func TestEvalIntCoercion(t *testing.T) {
	// JSON numbers decode to float64; an int param must be coerced to int64 to match cel-go.
	cond := &descope.FGACondition{
		Name:       "highClearance",
		Params:     []*descope.FGAConditionParam{{Name: "level", Type: "int"}},
		Expression: `level >= 5`,
	}
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
	cond := &descope.FGACondition{
		Name:       "inOfficeRange",
		Params:     []*descope.FGAConditionParam{{Name: "ip", Type: "ipaddress"}},
		Expression: `ip.in_cidr("10.0.0.0/8")`,
	}
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

func TestCompileInvalidExpression(t *testing.T) {
	_, err := edgecel.Compile(&descope.FGACondition{
		Name:       "broken",
		Params:     []*descope.FGAConditionParam{{Name: "role", Type: "string"}},
		Expression: `role ==`, // syntax error
	})
	require.Error(t, err)
}

func TestCompileUnknownParamType(t *testing.T) {
	_, err := edgecel.Compile(&descope.FGACondition{
		Name:       "weird",
		Params:     []*descope.FGAConditionParam{{Name: "x", Type: "nonexistent"}},
		Expression: `true`,
	})
	require.Error(t, err)
}
