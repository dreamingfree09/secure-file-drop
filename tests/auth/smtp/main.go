// Command smtp is an isolated fixture-only, authenticated loopback mail sink.
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/textproto"
	"os"
	"strings"
	"sync"
	"time"
)

var mailMu sync.Mutex

func handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	if conn.SetDeadline(time.Now().Add(10*time.Second)) != nil {
		return
	}
	reader := textproto.NewReader(bufio.NewReader(io.LimitReader(conn, 2<<20)))
	reply := func(s string) bool { _, e := io.WriteString(conn, s+"\r\n"); return e == nil }
	if !reply("220 localhost fixture SMTP") {
		return
	}
	authed := false
	recipient := ""
	for {
		line, e := reader.ReadLine()
		if e != nil {
			return
		}
		parts := strings.SplitN(line, " ", 2)
		verb := strings.ToUpper(parts[0])
		arg := ""
		if len(parts) == 2 {
			arg = parts[1]
		}
		switch verb {
		case "EHLO", "HELO":
			if !reply("250-localhost\r\n250 AUTH PLAIN") {
				return
			}
		case "AUTH":
			fields := strings.Fields(arg)
			if len(fields) != 2 || fields[0] != "PLAIN" {
				if !reply("504 fixture AUTH PLAIN required") {
					return
				}
				continue
			}
			raw, e := base64.StdEncoding.DecodeString(fields[1])
			values := strings.Split(string(raw), "\x00")
			authed = e == nil && len(values) == 3 && values[1] == os.Getenv("SFD_SMTP_USER") && values[2] == os.Getenv("SFD_SMTP_PASS")
			if !authed {
				if !reply("535 authentication failed") {
					return
				}
			} else if !reply("235 authenticated") {
				return
			}
		case "MAIL":
			if !authed {
				if !reply("530 authenticate first") {
					return
				}
			} else if !reply("250 sender accepted") {
				return
			}
		case "RCPT":
			if !authed {
				if !reply("530 authenticate first") {
					return
				}
				continue
			}
			recipient = strings.Trim(strings.TrimPrefix(arg, "TO:"), "<>")
			if !reply("250 recipient accepted") {
				return
			}
		case "DATA":
			if !authed || recipient == "" {
				if !reply("503 recipient required") {
					return
				}
				continue
			}
			if !reply("354 end with dot") {
				return
			}
			body, e := reader.ReadDotBytes()
			if e != nil {
				return
			}
			mailMu.Lock()
			f, e := os.OpenFile("/tmp/sfd-fixture-mail.jsonl", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
			if e == nil {
				e = json.NewEncoder(f).Encode(struct{ To, Body string }{recipient, string(body)})
				closeErr := f.Close()
				if e == nil {
					e = closeErr
				}
			}
			mailMu.Unlock()
			if e != nil {
				reply("451 fixture write failed")
				return
			}
			if !reply("250 queued") {
				return
			}
		case "QUIT":
			reply("221 goodbye")
			return
		case "RSET":
			recipient = ""
			if !reply("250 reset") {
				return
			}
		default:
			if !reply("250 ok") {
				return
			}
		}
	}
}
func main() {
	if os.Getenv("SFD_AUTH_SMTP_FIXTURE") != "owned-loopback-only" {
		os.Exit(2)
	}
	if len(os.Args) == 2 && os.Args[1] == "check" {
		c, e := net.DialTimeout("tcp", "127.0.0.1:2525", time.Second)
		if e != nil {
			os.Exit(5)
		}
		_ = c.Close()
		return
	}
	listener, err := net.Listen("tcp", "127.0.0.1:2525")
	if err != nil {
		os.Exit(3)
	}

	for {
		conn, e := listener.Accept()
		if e != nil {
			return
		}
		go handle(conn)
	}
}
