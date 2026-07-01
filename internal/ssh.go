package internal

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type SSHRPC struct {
	svr        string
	user, pass string
	conn       *ssh.Client
	session    *ssh.Session
	stdoutBuf  io.Reader
	stdinBuf   io.WriteCloser
}

func NewSSHRPC(sshsvr, user, pass string) (*SSHRPC, error) {
	r := new(SSHRPC)
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(pass),
		},
		Timeout:         10 * time.Second,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	r.pass = pass
	r.user = user
	r.svr = sshsvr

	// Need pseudo terminal if we want to have an SSH session
	// similar to what you have when you use a SSH client
	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}
	var err error
	r.conn, err = ssh.Dial("tcp", sshsvr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to make ssh connection to %v,%v", sshsvr, err)
	}
	r.session, err = r.conn.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to make ssh session to %v,%v", sshsvr, err)
	}
	// You can use session.Run() here but that only works
	// if you need a run a single command or you commands
	// are independent of each other.
	//Note: width can't be too small, otherwise the output will wrap around
	err = r.session.RequestPty("xterm", 800, 400, modes)
	if err != nil {
		return nil, fmt.Errorf("failed to request for pseudo terminal to %v,%v", sshsvr, err)
	}
	r.stdoutBuf, err = r.session.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to request for stdout pipe to %v,%v", sshsvr, err)
	}
	r.stdinBuf, err = r.session.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to request for stdin pipe to %v,%v", sshsvr, err)
	}
	err = r.session.Shell()
	if err != nil {
		log.Fatalf("failed to start shell: %s", err)
		return nil, fmt.Errorf("failed to start shell on %v,%v", sshsvr, err)
	}
	return r, nil
}

const (
	SROS_SSH_MD_ENDMARK      = "TiMOS"
	SROS_SSH_CLASSIC_ENDMARK = "ENDMARK"
	SRL_SSH_ENDMARK          = "SRLEND"
	SRL_SSH_PRE_SENTINEL     = "SRLPRE"
)

// ansiEscapeRE matches ANSI terminal escape sequences and bare carriage returns.
var ansiEscapeRE = regexp.MustCompile(`\x1b(?:\[[0-9;?]*[A-Za-z]|[@-Z\\-_]|\][^\a]*\a)|\r`)

// srlPromptRE matches SRL CLI prompt lines (context header or prompt with or
// without an echoed command), e.g. "--{ running }--[  ]--" or "A:srl-1# ".
var srlPromptRE = regexp.MustCompile(`^(--\{|A:[^#]+#)`)

// srosPromptRE matches SROS CLI prompt lines (classic or MD-CLI), e.g.
// "A:VSR# " or "A:admin@SRSIM# show version".
var srosPromptRE = regexp.MustCompile(`^A:[^#]+#`)

func stripAnsi(s string) string {
	return ansiEscapeRE.ReplaceAllString(s, "")
}

func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func (rpc *SSHRPC) Stop() {
	rpc.stdinBuf.Close()
	rpc.session.Close()
	rpc.conn.Close()
}

// SSHRunCommand runs a single command over SSH and returns combined stdout+stderr.
func SSHRunCommand(host, user, pass, cmd string) (string, error) {
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(pass),
		},
		Timeout:         10 * time.Second,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	conn, err := ssh.Dial("tcp", host, config)
	if err != nil {
		return "", fmt.Errorf("failed to make ssh connection to %v: %w", host, err)
	}
	defer conn.Close()
	session, err := conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to make ssh session to %v: %w", host, err)
	}
	defer session.Close()
	var buf bytes.Buffer
	session.Stdout = &buf
	session.Stderr = &buf
	err = session.Run(cmd)
	if err != nil {
		return buf.String(), fmt.Errorf("failed to run command on %v: %w", host, err)
	}
	return buf.String(), nil
}

func (rpc *SSHRPC) SRLExecuteCmd(cmds []string) (string, error) {
	_, err := fmt.Fprintln(rpc.stdinBuf, "environment pagination off")
	if err != nil {
		panic(err)
	}
	for _, c := range cmds {
		_, err = fmt.Fprintln(rpc.stdinBuf, c)
		if err != nil {
			panic(err)
		}
	}
	_, err = fmt.Fprintf(rpc.stdinBuf, "echo %v\n", SRL_SSH_ENDMARK)
	if err != nil {
		panic(err)
	}
	s := srlReadUntillPrefix(rpc.stdoutBuf)
	return s, nil
}

// this function is specific to TIMOS, via MDCLI or classic CLI
func (rpc *SSHRPC) SROSExecuteCmd(cmds []string, isMDCLI bool) (string, error) {
	var err error
	if isMDCLI {
		_, err = fmt.Fprintln(rpc.stdinBuf, "/environment more false")
	} else {
		_, err = fmt.Fprintln(rpc.stdinBuf, "/environment no more")
	}
	if err != nil {
		panic(err)
	}
	for _, c := range cmds {
		_, err = fmt.Fprintln(rpc.stdinBuf, c)
		if err != nil {
			panic(err)
		}
	}
	endmark := SROS_SSH_MD_ENDMARK

	if isMDCLI {
		_, err = fmt.Fprintln(rpc.stdinBuf, "/show version")
	} else {
		_, err = fmt.Fprintf(rpc.stdinBuf, "echo %v\n", SROS_SSH_CLASSIC_ENDMARK)
		endmark = SROS_SSH_CLASSIC_ENDMARK
	}
	if err != nil {
		panic(err)
	}
	time.Sleep(100 * time.Millisecond)
	s := srosReadUntillPrefix(rpc.stdoutBuf, endmark)
	return s, nil
}

// SROSExecCmdLogout sends env setup + cmd + "logout", reads all output until the
// session closes, then extracts only the command output (stripping MOTD, prompts,
// and ANSI escape codes). isMDCLI selects between MD-CLI and classic CLI env cmd.
func (rpc *SSHRPC) SROSExecCmdLogout(cmd string, isMDCLI bool) (string, error) {
	var err error
	if isMDCLI {
		_, err = fmt.Fprintln(rpc.stdinBuf, "/environment more false")
	} else {
		_, err = fmt.Fprintln(rpc.stdinBuf, "/environment no more")
	}
	if err != nil {
		return "", fmt.Errorf("write env cmd: %w", err)
	}
	if _, err = fmt.Fprintln(rpc.stdinBuf, cmd); err != nil {
		return "", fmt.Errorf("write cmd: %w", err)
	}
	if _, err = fmt.Fprintln(rpc.stdinBuf, "logout"); err != nil {
		return "", fmt.Errorf("write logout: %w", err)
	}

	scanner := bufio.NewScanner(rpc.stdoutBuf)
	var rawLines []string
	for scanner.Scan() {
		rawLines = append(rawLines, scanner.Text())
	}

	extracted := extractSROSCmdOutput(rawLines, cmd)
	return extracted, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// extractSROSCmdOutput finds the command's output in the collected lines by:
// 1. Finding the first prompt line that contains the user's command (echo of command)
// 2. Collecting lines until the next CLI prompt marker or end of output
// ANSI escape codes are stripped from lines before matching and output.
func extractSROSCmdOutput(lines []string, cmd string) string {
	start := -1
	end := len(lines)

	for i, raw := range lines {
		line := stripAnsi(raw)
		// Find the prompt line echoing the command, e.g. "A:admin@SRSIM# show version"
		if start < 0 && strings.Contains(line, cmd) && strings.Contains(line, "#") {
			start = i + 1
			continue
		}
		// After we've found the start, stop at the next CLI prompt marker
		if start >= 0 {
			clean := strings.TrimSpace(line)
			// Classic CLI prompt preamble "[/]", or MD-CLI prompt "(gl)["
			if clean == "[/]" || strings.HasPrefix(clean, "(gl)[") {
				end = i
				break
			}
		}
	}

	if start < 0 || start >= end {
		return ""
	}

	out := make([]string, 0, end-start)
	for _, raw := range lines[start:end] {
		line := strings.TrimRight(stripAnsi(raw), " \t")
		// Filter out SROS prompt lines (bare prompts and command echoes like
		// "A:VSR# logout") that bleed through when there is no end-marker
		// (classic CLI runs until session EOF).
		if srosPromptRE.MatchString(line) {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func srlReadUntillPrefix(input io.Reader) string {
	scanner := bufio.NewScanner(input)
	lineList := []string{}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, SRL_SSH_ENDMARK) {
			break
		}
		lineList = append(lineList, line)
	}
	lineList = lineList[:len(lineList)-2]
	return strings.Join(lineList, "\n")
}

// extractSRLOutput strips SRL prompt/context lines from lines collected after
// the PRE sentinel and returns only the actual command output.
// Both context lines ("--{ ... }--") and prompt lines ("A:hostname# ...") are
// removed; all other lines are kept with ANSI codes stripped.
func extractSRLOutput(lines []string) string {
	out := make([]string, 0, len(lines))
	for _, raw := range lines {
		line := strings.TrimRight(stripAnsi(raw), " \t")
		if srlPromptRE.MatchString(line) {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// SRLExecuteAndExtract sends a single command over an interactive SRL SSH
// session and returns only the command's output, stripping MOTD, prompts, and
// any command echoes.
//
// Strategy: emit "echo SRLPRE" before the real command so we can discard all
// output up to and including the SRLPRE line (MOTD, login banner, initial
// prompts). Everything between SRLPRE and the final SRLEND sentinel is the
// command output, from which prompt/context lines are then filtered.
func (rpc *SSHRPC) SRLExecuteAndExtract(cmd string) (string, error) {
	if _, err := fmt.Fprintln(rpc.stdinBuf, "environment pagination off"); err != nil {
		return "", fmt.Errorf("write pagination cmd: %w", err)
	}
	if _, err := fmt.Fprintf(rpc.stdinBuf, "echo %v\n", SRL_SSH_PRE_SENTINEL); err != nil {
		return "", fmt.Errorf("write pre-sentinel: %w", err)
	}
	if _, err := fmt.Fprintln(rpc.stdinBuf, cmd); err != nil {
		return "", fmt.Errorf("write cmd: %w", err)
	}
	if _, err := fmt.Fprintf(rpc.stdinBuf, "echo %v\n", SRL_SSH_ENDMARK); err != nil {
		return "", fmt.Errorf("write endmark: %w", err)
	}

	scanner := bufio.NewScanner(rpc.stdoutBuf)
	var rawLines []string
	foundPre := false
	for scanner.Scan() {
		line := scanner.Text()
		clean := stripAnsi(line)
		if strings.Contains(clean, SRL_SSH_ENDMARK) {
			break
		}
		if !foundPre {
			// Use exact match so the command-echo line ("A:srl1# echo SRLPRE")
			// does not prematurely set foundPre; only the bare sentinel output
			// line ("SRLPRE") should.
			if strings.TrimSpace(clean) == SRL_SSH_PRE_SENTINEL {
				foundPre = true
			}
			continue
		}
		rawLines = append(rawLines, line)
	}

	return extractSRLOutput(rawLines), nil
}

func srosReadUntillPrefix(input io.Reader, endmark string) string {
	scanner := bufio.NewScanner(input)
	lineList := []string{}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, endmark) {
			break
		}
		lineList = append(lineList, line)
	}
	return strings.Join(lineList, "\n")
}
