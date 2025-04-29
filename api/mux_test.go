package api

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

type exampleAction struct {
	output *bytes.Buffer
}

func (e exampleAction) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintln(e.output, "      in the action")
}

func firstAdapter(output *bytes.Buffer) Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(output, "firstAdapter before")
			next.ServeHTTP(w, r)
			fmt.Fprintln(output, "firstAdapter after")
		})
	}
}

func secondAdapter(output *bytes.Buffer) Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(output, "  secondAdapter before")
			next.ServeHTTP(w, r)
			fmt.Fprintln(output, "  secondAdapter after")
		})
	}
}

func thirdAdapter(output *bytes.Buffer) Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(output, "    thirdAdapter before")
			next.ServeHTTP(w, r)
			fmt.Fprintln(output, "    thirdAdapter after")
		})
	}
}

// TestAdapt demonstrates how the Adapter pattern works
func TestAdapt(t *testing.T) {
	b := bytes.Buffer{}
	Adapt(
		exampleAction{output: &b},
		firstAdapter(&b),
		secondAdapter(&b),
		thirdAdapter(&b),
	).ServeHTTP(nil, nil)
	require.Equal(t, ""+
		"firstAdapter before\n"+
		"  secondAdapter before\n"+
		"    thirdAdapter before\n"+
		"      in the action\n"+
		"    thirdAdapter after\n"+
		"  secondAdapter after\n"+
		"firstAdapter after\n",
		b.String(),
	)
}
