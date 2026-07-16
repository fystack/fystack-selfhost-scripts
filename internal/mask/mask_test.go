package mask

import (
	"strings"
	"testing"
)

func TestSensitiveMasksSecrets(t *testing.T) {
	input := `password: "plain"
ENCRYPTION_KEY=82441c6785f53e02dbf97db9db2107ad
private_key: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`
	got := Sensitive(input)
	for _, leaked := range []string{"plain", "82441c6785f53e02dbf97db9db2107ad", strings.Repeat("a", 64)} {
		if strings.Contains(got, leaked) {
			t.Fatalf("secret leaked in output:\n%s", got)
		}
	}
}
