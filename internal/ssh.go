package internal

import (
	"bufio"
	"fmt"
	"io"
	"log"
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
)

func (rpc *SSHRPC) Stop() {
	rpc.stdinBuf.Close()
	rpc.session.Close()
	rpc.conn.Close()
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
