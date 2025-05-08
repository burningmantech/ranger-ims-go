package redact_test

import (
	"github.com/burningmantech/ranger-ims-go/lib/redact"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

type ExampleType struct {
	SomeString string
	SomeNum    int
	Passwords  []string `redact:"true"`
	Secret     Secret   `redact:"true"`
	Secrets    []Secret `redact:"true"`
}

type Secret struct {
	Things []string
	PIN    int
}

func TestToBytes(t *testing.T) {
	t.Parallel()
	e := ExampleType{
		SomeString: "This is a string",
		SomeNum:    123456,
		Passwords: []string{
			"password1",
			"password2",
			"password3",
		},
		Secret: Secret{
			Things: []string{"abc"},
			PIN:    123,
		},
		Secrets: []Secret{{}, {}},
	}
	expected := `
SomeString = This is a string
SomeNum = 123456
Passwords = [🤐🤐🤐🤐]
Secret
    🤐🤐🤐🤐🤐
Secrets[0]
    🤐🤐
Secrets[1]
    🤐🤐`
	b, err := redact.ToBytes(&e)
	require.NoError(t, err)
	require.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(string(b)))
}

type ExampleType2 struct {
	MyMap map[string]string
}

func TestToBytes_noMapSupport(t *testing.T) {
	t.Parallel()
	// we haven't bothered adding support for various Kinds yet, but feel free to do so if the need arises!
	e := ExampleType2{}
	_, err := redact.ToBytes(&e)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported field kind: map")
}
