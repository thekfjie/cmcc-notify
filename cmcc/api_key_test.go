package cmcc

import "testing"

func TestExtractAPIKey(t *testing.T) {
	const key = "ak_01234567-89ab-cdef-0123-456789abcdef"
	for _, test := range []struct {
		name  string
		input string
		want  string
		ok    bool
	}{
		{name: "bare key", input: key, want: key, ok: true},
		{name: "trimmed bare key", input: "  " + key + "\n", want: key, ok: true},
		{
			name: "authorization message",
			input: "【新消息Claw】请根据 https://example.invalid/channel-guide.md 安装插件。" +
				"我的新消息Channel API Key为" + key,
			want: key,
			ok:   true,
		},
		{name: "app key compatibility", input: "Channel API Key: app_example-key", want: "app_example-key", ok: true},
		{name: "invalid text", input: "授权成功，但此消息不包含密钥", ok: false},
		{name: "missing value", input: "ak_", ok: false},
		{name: "embedded prefix", input: "prefixak_example", ok: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ok := ExtractAPIKey(test.input)
			if got != test.want || ok != test.ok {
				t.Fatalf("ExtractAPIKey() = %q, %t; want %q, %t", got, ok, test.want, test.ok)
			}
		})
	}
}

func TestNormalizeAndValidateAPIKey(t *testing.T) {
	const key = "ak_0123456789abcdef0123456789abcdef"
	message := "我的新消息Channel API Key为" + key + "。"
	if got := NormalizeAPIKey(message); got != key {
		t.Fatalf("NormalizeAPIKey() = %q", got)
	}
	if !ValidAPIKey(key) {
		t.Fatal("bare key should be valid")
	}
	if ValidAPIKey(message) || ValidAPIKey("invalid text") {
		t.Fatal("only normalized bare keys should validate")
	}
	client, err := NewClient(message, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if client.apiKey != key {
		t.Fatalf("NewClient stored %q", client.apiKey)
	}
}
