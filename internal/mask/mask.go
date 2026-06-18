package mask

import "regexp"

var patterns = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`badger_password:\s*"[^"]*"`), `badger_password: "***MASKED***"`},
	{regexp.MustCompile(`Generated BadgerDB password:\s*\S+`), `Generated BadgerDB password: ***MASKED***`},
	{regexp.MustCompile(`Password:\s*\S+`), `Password: ***MASKED***`},
	{regexp.MustCompile(`encryption_key:\s*[a-f0-9]{32}`), `encryption_key: ***MASKED***`},
	{regexp.MustCompile(`Found encryption key:\s*[a-f0-9]{32}`), `Found encryption key: ***MASKED***`},
	{regexp.MustCompile(`ENCRYPTION_KEY=[a-f0-9]{32}`), `ENCRYPTION_KEY=***MASKED***`},
	{regexp.MustCompile(`event_initiator_pubkey:\s*"[^"]*"`), `event_initiator_pubkey: "***MASKED***"`},
	{regexp.MustCompile(`Event initiator public key:\s*\S+`), `Event initiator public key: ***MASKED***`},
	{regexp.MustCompile(`event_initiator_pk_raw:\s*"[^"]*"`), `event_initiator_pk_raw: "***MASKED***"`},
	{regexp.MustCompile(`Event initiator private key length:\s*\d+\s*characters`), `Event initiator private key length: ***MASKED*** characters`},
	{regexp.MustCompile(`jwt_secret:\s*[a-f0-9]{32}`), `jwt_secret: ***MASKED***`},
	{regexp.MustCompile(`api_key:\s*"[^"]*"`), `api_key: "***MASKED***"`},
	{regexp.MustCompile(`api_secret:\s*"[^"]*"`), `api_secret: "***MASKED***"`},
	{regexp.MustCompile(`client_secret:\s*"[^"]*"`), `client_secret: "***MASKED***"`},
	{regexp.MustCompile(`password:\s*"[^"]*"`), `password: "***MASKED***"`},
	{regexp.MustCompile(`redis_password:\s*"[^"]*"`), `redis_password: "***MASKED***"`},
	{regexp.MustCompile(`private_key:\s*"[a-f0-9]{64}"`), `private_key: "***MASKED***"`},
	{regexp.MustCompile(`peer_id:\s*"[^"]*"`), `peer_id: "***MASKED***"`},
}

func Sensitive(text string) string {
	for _, pattern := range patterns {
		text = pattern.re.ReplaceAllString(text, pattern.repl)
	}
	return text
}
