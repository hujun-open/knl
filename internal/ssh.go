package internal

import (
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

var SROS_CLI_PROMPT_PREFIXES = []string{"A:", "*A:"}

const SSH_ENDMARK = "TiMOS"

func splitlines(inputstr string, trim bool) []string {
	r := strings.Split(inputstr, "\n")
	var r2 []string
	for _, s := range r {
		s3 := strings.TrimRight(s, "\n\r")
		if trim {
			s3 = strings.TrimSpace(s3)
		}
		r2 = append(r2, s3)
	}
	return r2
}

func (rpc *SSHRPC) Stop() {
	rpc.stdinBuf.Close()
	rpc.session.Close()
	rpc.conn.Close()
}

// this function is specific to TIMOS
func (rpc *SSHRPC) ExecuteCmd(cmds []string) (string, error) {
	_, err := fmt.Fprintln(rpc.stdinBuf, "/environment more false")
	if err != nil {
		panic(err)
	}
	for _, c := range cmds {
		_, err = fmt.Fprintln(rpc.stdinBuf, c)
		if err != nil {
			panic(err)
		}
	}
	_, err = fmt.Fprintln(rpc.stdinBuf, "/show version")
	if err != nil {
		panic(err)
	}
	// _, err := rpc.stdinBuf.Write([]byte(cmd))
	// if err != nil {
	// 	return "", err
	// }
	time.Sleep(100 * time.Millisecond)
	s := readUntillPrefix(rpc.stdoutBuf, SROS_CLI_PROMPT_PREFIXES)
	return s, nil
}

func readUntillPrefix(input io.Reader, prefixstrs []string) string {
	const MAXLEN = 1000000
	readbuf := make([]byte, 100000)
	rstr := ""
	gotend := false
	for {
		byteCount, err := input.Read(readbuf)
		if err != nil {
			log.Fatal(err)
		}
		cur_str := string(readbuf[:byteCount])
		//fmt.Println("cur_str:", cur_str)
		rstr += cur_str
		line_list := splitlines(cur_str, false)
		for _, line := range line_list {
			if strings.Contains(line, SSH_ENDMARK) {
				gotend = true
				break
			}
		}
		if gotend {
			lastline := line_list[len(line_list)-1]
			checkline := strings.TrimSpace(lastline)
			if matchPrefix(checkline, prefixstrs) {
				break
			}
		}

		if len(rstr) >= MAXLEN {
			log.Fatal("ssh output exceed max length, abort")
		}
	}
	return rstr

}
func matchPrefix(inpustr string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(inpustr, p) {
			return true
		}
	}
	return false
}
