// Package java contains optional Java-language-service infrastructure. It has
// no dependency on the document model so it can remain lazy and failure
// isolated from the reader.
package java

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

var (
	ErrClosed        = errors.New("java: language client is closed")
	ErrStaleResponse = errors.New("java: stale language-server response")
	ErrUntrusted     = errors.New("java: workspace is not trusted")
	ErrPolicy        = errors.New("java: start policy does not permit language server")
)

// Position is an LSP position: line number plus UTF-16 code-unit offset.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// ByteOffsetToPosition converts a UTF-8 offset into an LSP UTF-16 position.
// A CR preceding a newline is not part of either line's addressable content.
func ByteOffsetToPosition(text string, offset int) (Position, error) {
	if offset < 0 || offset > len(text) {
		return Position{}, fmt.Errorf("java: byte offset %d out of range", offset)
	}
	line, character := 0, 0
	for i := 0; i < len(text); {
		if i == offset {
			return Position{line, character}, nil
		}
		if text[i] == '\n' {
			i++
			line++
			character = 0
			continue
		}
		if text[i] == '\r' && i+1 < len(text) && text[i+1] == '\n' {
			if offset == i {
				return Position{line, character}, nil
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && size == 1 {
			return Position{}, fmt.Errorf("java: invalid UTF-8 at byte %d", i)
		}
		if offset > i && offset < i+size {
			return Position{}, fmt.Errorf("java: byte offset %d splits UTF-8 sequence", offset)
		}
		if r > 0xFFFF {
			character += 2
		} else {
			character++
		}
		i += size
	}
	if offset == len(text) {
		return Position{line, character}, nil
	}
	return Position{}, fmt.Errorf("java: byte offset %d is invalid", offset)
}

// PositionToByteOffset converts an LSP position back to a UTF-8 boundary.
func PositionToByteOffset(text string, position Position) (int, error) {
	if position.Line < 0 || position.Character < 0 {
		return 0, errors.New("java: negative LSP position")
	}
	line, character := 0, 0
	for i := 0; ; {
		if line == position.Line && character == position.Character {
			return i, nil
		}
		if i == len(text) {
			break
		}
		if text[i] == '\n' {
			if line == position.Line {
				break
			}
			i++
			line++
			character = 0
			continue
		}
		if text[i] == '\r' && i+1 < len(text) && text[i+1] == '\n' {
			if line == position.Line {
				break
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && size == 1 {
			return 0, fmt.Errorf("java: invalid UTF-8 at byte %d", i)
		}
		width := 1
		if r > 0xFFFF {
			width = 2
		}
		if line == position.Line && position.Character > character && position.Character < character+width {
			return 0, fmt.Errorf("java: character %d splits UTF-16 surrogate pair", position.Character)
		}
		character += width
		i += size
	}
	return 0, fmt.Errorf("java: LSP position %+v out of range", position)
}

// StartPolicy makes language-service activation explicit.
type StartPolicy uint8

const (
	Never StartPolicy = iota
	Ask
	OnNavigation
	AutomaticTrusted
)

func (p StartPolicy) ShouldStart(trusted, navigation bool) bool {
	switch p {
	case AutomaticTrusted:
		return trusted
	case OnNavigation:
		return trusted && navigation
	default:
		return false
	}
}

// ProjectKind identifies markers observed without executing their contents.
type ProjectKind string

const (
	Standalone ProjectKind = "standalone"
	Maven      ProjectKind = "maven"
	Gradle     ProjectKind = "gradle"
)

type Project struct {
	Root    string
	Kind    ProjectKind
	Markers []string
}

// TrustStore records explicit workspace trust decisions. A decision is scoped
// to the canonical directory path and never stores workspace-provided commands.
type TrustStore struct {
	mu      sync.RWMutex
	trusted map[string]bool
}

func NewTrustStore() *TrustStore { return &TrustStore{trusted: make(map[string]bool)} }

func canonicalWorkspace(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func (s *TrustStore) Set(path string, trusted bool) error {
	canonical, err := canonicalWorkspace(path)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.trusted == nil {
		s.trusted = make(map[string]bool)
	}
	if trusted {
		s.trusted[canonical] = true
	} else {
		delete(s.trusted, canonical)
	}
	return nil
}

func (s *TrustStore) IsTrusted(path string) bool {
	canonical, err := canonicalWorkspace(path)
	if err != nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.trusted[canonical]
}

var projectMarkers = []struct {
	Name string
	Kind ProjectKind
}{
	{"pom.xml", Maven}, {"build.gradle", Gradle}, {"build.gradle.kts", Gradle},
	{"settings.gradle", Gradle}, {"settings.gradle.kts", Gradle},
}

// DetectProject looks only for known project marker filenames in root.
func DetectProject(root string) (Project, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Project{}, err
	}
	p := Project{Root: filepath.Clean(abs), Kind: Standalone}
	for _, marker := range projectMarkers {
		info, err := os.Stat(filepath.Join(p.Root, marker.Name))
		if err == nil && !info.IsDir() {
			p.Markers = append(p.Markers, marker.Name)
			if marker.Kind == Maven {
				p.Kind = Maven
			} else if p.Kind != Maven {
				p.Kind = Gradle
			}
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return Project{}, err
		}
	}
	return p, nil
}

// FindProject walks ancestors and chooses the nearest directory containing a
// Java project marker. It never reads or executes a build file.
func FindProject(start string) (Project, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return Project{}, err
	}
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	for {
		p, err := DetectProject(abs)
		if err != nil {
			return Project{}, err
		}
		if p.Kind != Standalone {
			return p, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return Project{Root: filepath.Clean(start), Kind: Standalone}, nil
		}
		abs = parent
	}
}

type JDK struct {
	Home    string
	Java    string
	Version string
}

// DiscoverJDKs inspects provided installation roots and JAVA_HOME. It reads
// release metadata rather than starting Java, so opening a workspace cannot
// execute project-controlled code.
func DiscoverJDKs(roots []string) ([]JDK, error) {
	if home := os.Getenv("JAVA_HOME"); home != "" {
		roots = append([]string{home}, roots...)
	}
	seen := make(map[string]bool)
	result := make([]JDK, 0)
	for _, root := range roots {
		abs, err := filepath.Abs(root)
		if err != nil {
			return nil, err
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		jdk, ok, err := inspectJDK(abs)
		if err != nil {
			return nil, err
		}
		if ok {
			result = append(result, jdk)
		}
		entries, err := os.ReadDir(abs)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			home := filepath.Join(abs, entry.Name())
			if seen[home] {
				continue
			}
			seen[home] = true
			jdk, ok, err := inspectJDK(home)
			if err != nil {
				return nil, err
			}
			if ok {
				result = append(result, jdk)
			}
		}
	}
	return result, nil
}

func inspectJDK(home string) (JDK, bool, error) {
	java := filepath.Join(home, "bin", "java")
	if _, err := os.Stat(java); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return JDK{}, false, nil
		}
		return JDK{}, false, err
	}
	version := ""
	if release, err := os.ReadFile(filepath.Join(home, "release")); err == nil {
		for _, line := range strings.Split(string(release), "\n") {
			if value, ok := strings.CutPrefix(line, "JAVA_VERSION="); ok {
				version = strings.Trim(value, "\"")
				break
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return JDK{}, false, err
	}
	return JDK{Home: home, Java: java, Version: version}, true, nil
}

// Message is the JSON-RPC 2.0 envelope used by LSP framing.
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ResponseError  `json:"error,omitempty"`
}

type ResponseError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *ResponseError) Error() string {
	return fmt.Sprintf("java: LSP error %d: %s", e.Code, e.Message)
}

const DefaultMaxMessageSize = 16 << 20

// ReadMessage reads exactly one LSP Content-Length framed JSON message.
func ReadMessage(r *bufio.Reader, maxSize int) (Message, error) {
	if maxSize <= 0 {
		maxSize = DefaultMaxMessageSize
	}
	length := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return Message{}, err
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if line == "" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return Message{}, fmt.Errorf("java: malformed LSP header %q", line)
		}
		if strings.EqualFold(strings.TrimSpace(key), "Content-Length") {
			v, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || v < 0 || v > maxSize {
				return Message{}, fmt.Errorf("java: invalid Content-Length %q", value)
			}
			length = v
		}
	}
	if length < 0 {
		return Message{}, errors.New("java: missing Content-Length")
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return Message{}, err
	}
	var message Message
	if err := json.Unmarshal(payload, &message); err != nil {
		return Message{}, fmt.Errorf("java: malformed JSON-RPC payload: %w", err)
	}
	if message.JSONRPC != "2.0" {
		return Message{}, errors.New("java: expected JSON-RPC 2.0")
	}
	return message, nil
}

// WriteMessage serializes one LSP frame. The mutex lives at the client level;
// this helper is also useful to deterministic fake servers.
func WriteMessage(w io.Writer, message Message) error {
	message.JSONRPC = "2.0"
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(payload))
	if err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}

type RequestOptions struct {
	Timeout  time.Duration
	StaleKey string
	Revision uint64
}

type ClientOptions struct {
	DefaultTimeout  time.Duration
	MaxMessageSize  int
	OnNotification  func(Message)
	OnProtocolError func(error)
}

type pendingRequest struct {
	result   chan callResult
	staleKey string
	revision uint64
}
type callResult struct {
	message Message
	err     error
}

// Client is a concurrent JSON-RPC client over a language server's stdio.
type Client struct {
	in        *bufio.Reader
	inCloser  io.Closer
	out       io.Writer
	options   ClientOptions
	writeMu   sync.Mutex
	mu        sync.Mutex
	nextID    int64
	pending   map[int64]pendingRequest
	latest    map[string]uint64
	closed    bool
	done      chan struct{}
	closeOnce sync.Once
}

func NewClient(in io.Reader, out io.Writer, options ClientOptions) *Client {
	c := &Client{in: bufio.NewReader(in), out: out, options: options, pending: make(map[int64]pendingRequest), latest: make(map[string]uint64), done: make(chan struct{})}
	if closer, ok := in.(io.Closer); ok {
		c.inCloser = closer
	}
	go c.readLoop()
	return c
}

func (c *Client) readLoop() {
	for {
		message, err := ReadMessage(c.in, c.options.MaxMessageSize)
		if err != nil {
			c.fail(err)
			return
		}
		if len(message.ID) == 0 {
			if c.options.OnNotification != nil {
				c.options.OnNotification(message)
			}
			continue
		}
		var id int64
		if err := json.Unmarshal(message.ID, &id); err != nil {
			continue
		}
		c.mu.Lock()
		pending, ok := c.pending[id]
		if ok {
			delete(c.pending, id)
		}
		stale := ok && pending.staleKey != "" && c.latest[pending.staleKey] != pending.revision
		c.mu.Unlock()
		if ok {
			if stale {
				pending.result <- callResult{err: ErrStaleResponse}
			} else if message.Error != nil {
				pending.result <- callResult{err: message.Error}
			} else {
				pending.result <- callResult{message: message}
			}
		}
	}
}

func (c *Client) fail(err error) {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		pending := c.pending
		c.pending = make(map[int64]pendingRequest)
		c.mu.Unlock()
		for _, request := range pending {
			request.result <- callResult{err: err}
		}
		if c.options.OnProtocolError != nil && !errors.Is(err, io.EOF) {
			c.options.OnProtocolError(err)
		}
		if c.inCloser != nil {
			_ = c.inCloser.Close()
		}
		close(c.done)
	})
}

func (c *Client) send(message Message) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return ErrClosed
	}
	if err := WriteMessage(c.out, message); err != nil {
		c.fail(err)
		return err
	}
	return nil
}

func (c *Client) Notify(method string, params any) error {
	payload, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return c.send(Message{Method: method, Params: payload})
}

// Request executes one request and returns its raw JSON result. Cancelling the
// context sends $/cancelRequest and permanently rejects a later response.
func (c *Client) Request(ctx context.Context, method string, params any, options RequestOptions) (json.RawMessage, error) {
	if options.Timeout <= 0 {
		options.Timeout = c.options.DefaultTimeout
	}
	if options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, options.Timeout)
		defer cancel()
	}
	payload, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrClosed
	}
	c.nextID++
	id := c.nextID
	if options.StaleKey != "" && options.Revision >= c.latest[options.StaleKey] {
		c.latest[options.StaleKey] = options.Revision
	}
	result := make(chan callResult, 1)
	c.pending[id] = pendingRequest{result: result, staleKey: options.StaleKey, revision: options.Revision}
	c.mu.Unlock()
	if err := c.send(Message{ID: json.RawMessage(strconv.FormatInt(id, 10)), Method: method, Params: payload}); err != nil {
		c.removePending(id)
		return nil, err
	}
	select {
	case received := <-result:
		if received.err != nil {
			return nil, received.err
		}
		return received.message.Result, nil
	case <-ctx.Done():
		if c.removePending(id) {
			// A server can stop reading while it computes a request. Cancellation
			// must therefore never make the reader/UI wait on a blocked stdin.
			go func() { _ = c.Notify("$/cancelRequest", map[string]int64{"id": id}) }()
		}
		return nil, ctx.Err()
	case <-c.done:
		return nil, ErrClosed
	}
}

func (c *Client) removePending(id int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.pending[id]; ok {
		delete(c.pending, id)
		return true
	}
	return false
}
func (c *Client) Done() <-chan struct{} { return c.done }
func (c *Client) Close()                { c.fail(ErrClosed) }

// ProcessHooks allow the shell to surface lifecycle state without coupling the
// language client to UI code.
type ProcessHooks struct {
	OnStart func()
	OnExit  func(error)
}

// BoundedLog is a concurrency-safe, line-oriented stderr sink. Language
// servers can be exceptionally noisy; retaining a fixed recent tail gives
// diagnostics without allowing a failed optional service to consume memory.
type BoundedLog struct {
	mu       sync.RWMutex
	capacity int
	lines    []string
	partial  string
}

func NewBoundedLog(capacity int) *BoundedLog {
	if capacity <= 0 {
		capacity = 256
	}
	return &BoundedLog{capacity: capacity}
}

func (l *BoundedLog) Write(data []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.partial += string(data)
	for {
		line, rest, found := strings.Cut(l.partial, "\n")
		if !found {
			break
		}
		l.appendLocked(strings.TrimSuffix(line, "\r"))
		l.partial = rest
	}
	return len(data), nil
}

func (l *BoundedLog) appendLocked(line string) {
	if len(l.lines) == l.capacity {
		copy(l.lines, l.lines[1:])
		l.lines[len(l.lines)-1] = line
		return
	}
	l.lines = append(l.lines, line)
}

func (l *BoundedLog) Lines() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	lines := append([]string(nil), l.lines...)
	if l.partial != "" {
		lines = append(lines, l.partial)
	}
	return lines
}

type Supervisor struct {
	Command       []string
	Dir           string
	Env           []string // explicit allowlist; nil leaves Go's default environment
	Policy        StartPolicy
	Trusted       bool
	ClientOptions ClientOptions
	Logs          *BoundedLog
	Hooks         ProcessHooks
	mu            sync.Mutex
	cmd           *exec.Cmd
	client        *Client
}

// Start starts exactly one trusted language-server process. Command is an
// argument array; it is never interpreted by a shell.
func (s *Supervisor) Start(ctx context.Context, navigation bool) (*Client, error) {
	if !s.Trusted {
		return nil, ErrUntrusted
	}
	if !s.Policy.ShouldStart(s.Trusted, navigation) {
		return nil, ErrPolicy
	}
	if len(s.Command) == 0 {
		return nil, errors.New("java: empty language-server command")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		return s.client, nil
	}
	cmd := exec.CommandContext(ctx, s.Command[0], s.Command[1:]...)
	cmd.Dir = s.Dir
	if s.Env != nil {
		cmd.Env = append([]string(nil), s.Env...)
	}
	if s.Logs == nil {
		s.Logs = NewBoundedLog(256)
	}
	cmd.Stderr = s.Logs
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	s.cmd = cmd
	s.client = NewClient(stdout, stdin, s.ClientOptions)
	if s.Hooks.OnStart != nil {
		s.Hooks.OnStart()
	}
	go s.wait(cmd, s.client)
	return s.client, nil
}

func (s *Supervisor) wait(cmd *exec.Cmd, client *Client) {
	err := cmd.Wait()
	client.Close()
	s.mu.Lock()
	if s.cmd == cmd {
		s.cmd, s.client = nil, nil
	}
	s.mu.Unlock()
	if s.Hooks.OnExit != nil {
		s.Hooks.OnExit(err)
	}
}

func (s *Supervisor) Close() error {
	s.mu.Lock()
	cmd, client := s.cmd, s.client
	s.cmd, s.client = nil, nil
	s.mu.Unlock()
	if client != nil {
		client.Close()
	}
	if cmd != nil && cmd.Process != nil {
		return cmd.Process.Kill()
	}
	return nil
}
