package java

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestUTF16PositionRoundTrip(t *testing.T) {
	text := "ASCII 😀\r\nélan\n"
	for _, offset := range []int{0, 1, 6, 10, 12, len(text)} {
		position, err := ByteOffsetToPosition(text, offset)
		if err != nil {
			t.Fatalf("offset %d: %v", offset, err)
		}
		got, err := PositionToByteOffset(text, position)
		if err != nil {
			t.Fatalf("position %+v: %v", position, err)
		}
		if got != offset && !(offset == 10 && got == 10) {
			t.Fatalf("offset %d -> %+v -> %d", offset, position, got)
		}
	}
	if _, err := PositionToByteOffset("😀", Position{Character: 1}); err == nil {
		t.Fatal("accepted a surrogate-half position")
	}
	if _, err := ByteOffsetToPosition("😀", 1); err == nil {
		t.Fatal("accepted a UTF-8 interior offset")
	}
}

func TestProjectJDKAndTrustDetection(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pom.xml"), []byte("not executed"), 0o600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	p, err := FindProject(nested)
	if err != nil || p.Root != root || p.Kind != Maven {
		t.Fatalf("project = %#v, %v", p, err)
	}
	javaHome := filepath.Join(root, "jdk-21")
	if err := os.MkdirAll(filepath.Join(javaHome, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(javaHome, "bin", "java"), nil, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(javaHome, "release"), []byte("JAVA_VERSION=\"21.0.2\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	jdks, err := DiscoverJDKs([]string{root})
	if err != nil || len(jdks) != 1 || jdks[0].Version != "21.0.2" {
		t.Fatalf("jdks = %#v, %v", jdks, err)
	}
	trust := NewTrustStore()
	if trust.IsTrusted(root) {
		t.Fatal("workspace unexpectedly trusted")
	}
	if err := trust.Set(root, true); err != nil || !trust.IsTrusted(root) {
		t.Fatalf("trust = %v", err)
	}
	if !AutomaticTrusted.ShouldStart(true, false) || OnNavigation.ShouldStart(true, false) || Ask.ShouldStart(true, true) {
		t.Fatal("unexpected start policy")
	}
}

func TestFraming(t *testing.T) {
	var wire bytes.Buffer
	if err := WriteMessage(&wire, Message{Method: "x", Params: json.RawMessage(`{"n":1}`)}); err != nil {
		t.Fatal(err)
	}
	message, err := ReadMessage(bufio.NewReader(&wire), 1024)
	if err != nil || message.Method != "x" || string(message.Params) != `{"n":1}` {
		t.Fatalf("message = %#v, %v", message, err)
	}
	if _, err := ReadMessage(bufio.NewReader(bytes.NewBufferString("Content-Length: not-number\r\n\r\n")), 1024); err == nil {
		t.Fatal("accepted malformed length")
	}
}

// pipeServer is a deterministic fake LSP transport. Tests control every
// response, including deliberately out-of-order ones.
type pipeServer struct {
	fromClient *bufio.Reader
	toClient   *io.PipeWriter
}

func newPipeClient(t *testing.T) (*Client, *pipeServer, func()) {
	t.Helper()
	fromClient, clientWriter := io.Pipe()
	clientReader, toClient := io.Pipe()
	client := NewClient(clientReader, clientWriter, ClientOptions{})
	return client, &pipeServer{fromClient: bufio.NewReader(fromClient), toClient: toClient}, func() { client.Close(); _ = clientWriter.Close(); _ = toClient.Close() }
}

func (s *pipeServer) receive(t *testing.T) Message {
	t.Helper()
	m, err := ReadMessage(s.fromClient, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func (s *pipeServer) respond(t *testing.T, id json.RawMessage, value string) {
	t.Helper()
	if err := WriteMessage(s.toClient, Message{ID: id, Result: json.RawMessage(value)}); err != nil {
		t.Fatal(err)
	}
}

func TestClientRejectsStaleOutOfOrderResponse(t *testing.T) {
	client, server, closeClient := newPipeClient(t)
	defer closeClient()
	var wg sync.WaitGroup
	results := make(chan error, 2)
	request := func(revision uint64) {
		defer wg.Done()
		_, err := client.Request(context.Background(), "textDocument/hover", map[string]string{"x": "y"}, RequestOptions{StaleKey: "doc", Revision: revision})
		results <- err
	}
	wg.Add(2)
	go request(1)
	first := server.receive(t)
	go request(2)
	second := server.receive(t)
	server.respond(t, second.ID, `{"new":true}`)
	server.respond(t, first.ID, `{"old":true}`)
	wg.Wait()
	var got []error
	for range 2 {
		got = append(got, <-results)
	}
	if !(errors.Is(got[0], ErrStaleResponse) || errors.Is(got[1], ErrStaleResponse)) {
		t.Fatalf("results = %v", got)
	}
}

func TestClientCancellationSendsProtocolCancellation(t *testing.T) {
	client, server, closeClient := newPipeClient(t)
	defer closeClient()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { _, err := client.Request(ctx, "slow", nil, RequestOptions{}); result <- err }()
	request := server.receive(t)
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("request error = %v", err)
	}
	cancellation := server.receive(t)
	if cancellation.Method != "$/cancelRequest" {
		t.Fatalf("notification = %#v", cancellation)
	}
	server.respond(t, request.ID, `null`) // late response is ignored
}

func TestSupervisorRefusesUnsafeStarts(t *testing.T) {
	s := &Supervisor{Command: []string{"definitely-not-a-server"}, Trusted: false, Policy: AutomaticTrusted}
	if _, err := s.Start(context.Background(), false); !errors.Is(err, ErrUntrusted) {
		t.Fatalf("error = %v", err)
	}
	s.Trusted, s.Policy = true, Never
	if _, err := s.Start(context.Background(), true); !errors.Is(err, ErrPolicy) {
		t.Fatalf("error = %v", err)
	}
}

func BenchmarkByteOffsetToPosition(b *testing.B) {
	text := "line 😀 café\n"
	for i := 0; i < 1024; i++ {
		text += text
	}
	offset := len(text) - 4
	b.ReportAllocs()
	for b.Loop() {
		_, _ = ByteOffsetToPosition(text, offset)
	}
}

func BenchmarkLSPFraming(b *testing.B) {
	var wire bytes.Buffer
	_ = WriteMessage(&wire, Message{Method: "ping", Params: json.RawMessage(`{}`)})
	payload := wire.Bytes()
	b.ReportAllocs()
	for b.Loop() {
		_, _ = ReadMessage(bufio.NewReader(bytes.NewReader(payload)), 1024)
	}
}
