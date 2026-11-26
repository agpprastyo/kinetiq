package config

import "testing"

func TestValidateRejectsDefaultSecretsInProduction(t *testing.T) {
	cases := []struct {
		env, secret string
		wantErr     bool
	}{
		{"production", "", true},
		{"production", "test-internal-secret", true},
		{"production", "a-real-secret", false},
		{"development", "test-internal-secret", false},
	}
	for _, tc := range cases {
		err := (&Config{AppEnv: tc.env, AuthInternalSecret: tc.secret}).Validate()
		if (err != nil) != tc.wantErr {
			t.Errorf("env=%q secret=%q: err=%v wantErr=%v", tc.env, tc.secret, err, tc.wantErr)
		}
	}
}

func TestAllowedWebOriginsParsesCommaList(t *testing.T) {
	got := (&Config{WebOrigin: " https://a.example.com, https://b.example.com ,"}).AllowedWebOrigins()
	if len(got) != 2 || got[0] != "https://a.example.com" || got[1] != "https://b.example.com" {
		t.Fatalf("unexpected origins: %v", got)
	}
}

func TestValidateRejectsWildcardWebOrigin(t *testing.T) {
	if err := (&Config{AppEnv: "development", WebOrigin: "*"}).Validate(); err == nil {
		t.Fatal("wildcard WEB_ORIGIN must be rejected")
	}
	if err := (&Config{AppEnv: "development", WebOrigin: "https://app.example.com"}).Validate(); err != nil {
		t.Fatal(err)
	}
}
