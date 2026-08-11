package cli

import "testing"

func TestCredentialOnlyAccountReauth(t *testing.T) {
	const controlURL = "http://control.example.test:60090"
	tests := []struct {
		name        string
		args        []string
		haveNodeKey bool
		want        bool
	}{
		{
			name:        "credentials preserve existing preferences",
			args:        []string{"--login-server=" + controlURL, "--username=rd", "--password-file=/run/secret", "--timeout=60s"},
			haveNodeKey: true,
			want:        true,
		},
		{
			name:        "new node uses full up flow",
			args:        []string{"--login-server=" + controlURL, "--username=rd", "--password-file=/run/secret"},
			haveNodeKey: false,
		},
		{
			name:        "changed control server uses full up flow",
			args:        []string{"--login-server=http://other.example.test:60090", "--username=rd", "--password-file=/run/secret"},
			haveNodeKey: true,
		},
		{
			name:        "explicit network preference uses full up flow",
			args:        []string{"--login-server=" + controlURL, "--username=rd", "--password-file=/run/secret", "--accept-routes"},
			haveNodeKey: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var upArgs upArgsT
			flags := newUpFlagSet("linux", &upArgs, "up")
			if err := flags.Parse(tt.args); err != nil {
				t.Fatalf("parsing flags: %v", err)
			}
			got := isCredentialOnlyAccountReauth(flags, upArgs, tt.haveNodeKey, controlURL)
			if got != tt.want {
				t.Fatalf("isCredentialOnlyAccountReauth() = %v, want %v", got, tt.want)
			}
		})
	}
}
