package main

import (
	"bufio"
	"bytes"
	"fmt"
	"golang.org/x/crypto/ssh"
	"io"
	"log"
	"net"
	"strings"
)

var (
	cmdBuffer bytes.Buffer
)

func handleConnection(nConn net.Conn, config *ssh.ServerConfig) {
	sshConn, chans, reqs, err := ssh.NewServerConn(nConn, config)
	if err != nil {
		log.Printf("Failed to handshake: %s", err)
		return
	}
	defer sshConn.Close()

	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			log.Printf("Could not accept channel: %s", err)
			continue
		}

		go handleRequests(channel, requests)
	}
}

func handleRequests(channel ssh.Channel, requests <-chan *ssh.Request) {
	defer channel.Close()

	for req := range requests {
		switch req.Type {
		case "shell":
			handleShell(channel)
		case "exec":
			handleExec(channel, req)
		case "pty-req":
			handlePtyReq(channel, req)
		case "env":
			handleEnv(channel, req)
		case "window-change":
			handleWindowChange(channel, req)
		default:
			channel.Close()
		}
	}
}

func handleShell(channel ssh.Channel) {
	channel.Write([]byte("Mock shell started. Type 'exit' to close. \r\n$ "))
	reader := bufio.NewReader(channel)
	cursorPos := 0
	for {
		b, err := reader.ReadByte()
		if err != nil {
			if err != io.EOF {
				log.Printf("Error reading from channel: %s", err)
			}
			break
		}

		switch b {
		case '\r', '\n':
			channel.Write([]byte("\r\n"))
			if cmdBuffer.String() == "exit" {
				channel.Write([]byte("Goodbye!\n"))
				return
			}
			handleEchoCommand(channel, cmdBuffer.String())
			channel.Write([]byte("$ "))
			cmdBuffer.Reset()
			cursorPos = 0

		case 0x7f, 0x08: // Backspace and delete
			if cursorPos > 0 && cmdBuffer.Len() > 0 {
				leftPart := cmdBuffer.String()[:cursorPos-1]
				rightPart := cmdBuffer.String()[cursorPos:]
				cmdBuffer.Reset()
				cmdBuffer.WriteString(leftPart + rightPart)
				cursorPos--
				channel.Write([]byte("\b \b"))
				channel.Write([]byte(rightPart + " "))
				channel.Write([]byte("\b" + strings.Repeat("\b", len(rightPart))))
			}

		case 0x03: // Ctrl+C
			cmdBuffer.Reset()
			channel.Write([]byte("^C\r\n$ "))

		case 0x04: // Ctrl+D
			channel.Write([]byte("\r\nlogout\r\n"))
			return

		case 0x0C: // Ctrl+L
			channel.Write([]byte("\033[H\033[2J$ "))

		case 0x1b: // Escape sequences
			if err := handleEscapeSequence(channel, reader, &cmdBuffer, &cursorPos); err != nil {
				log.Printf("Error handling escape sequence: %s", err)
				return
			}

		default:
			if cursorPos == cmdBuffer.Len() {
				cmdBuffer.WriteByte(b)
			} else {
				leftPart := cmdBuffer.String()[:cursorPos]
				rightPart := cmdBuffer.String()[cursorPos:]
				cmdBuffer.Reset()
				cmdBuffer.WriteString(leftPart + string(b) + rightPart)
			}
			cursorPos++
			channel.Write([]byte{b})
		}
	}
}

func handleEscapeSequence(channel ssh.Channel, reader *bufio.Reader, cmdBuffer *bytes.Buffer, cursorPos *int) error {
	seq1, err := reader.ReadByte()
	if err != nil {
		return err
	}
	if seq1 != '[' {
		return nil // Not an escape sequence we care about
	}

	seq2, err := reader.ReadByte()
	if err != nil {
		return err
	}

	switch seq2 {
	case 'C': // Right arrow
		if *cursorPos < cmdBuffer.Len() {
			*cursorPos++
			channel.Write([]byte("\x1b[C"))
		}
	case 'D': // Left arrow
		if *cursorPos > 0 {
			*cursorPos--
			channel.Write([]byte("\x1b[D"))
		}
	}

	return nil
}

func handleExec(channel ssh.Channel, req *ssh.Request) {
	cmd := string(req.Payload[4:])
	handleEchoCommand(channel, cmd)
	channel.SendRequest("exit-status", false, ssh.Marshal(&struct{ uint32 }{0}))
	channel.Close()
}

func handleEchoCommand(channel ssh.Channel, input string) {
	const prefix = "echo "
	fmt.Println("start:", input, ":end")
	if len(input) >= len(prefix) && input[:len(prefix)] == prefix {
		channel.Write([]byte(fmt.Sprintf("%s\r\n", input[len(prefix):])))
	} else {
		channel.Write([]byte(fmt.Sprintf("Unsupported Command: %s\r\n", input)))
	}
}

func handlePtyReq(channel ssh.Channel, req *ssh.Request) {
	// Parse the requested PTY configuration.
	termLen := req.Payload[3]
	termEnv := string(req.Payload[4 : 4+termLen])
	width := req.Payload[4+termLen:]
	height := width[4:]
	widthInt := width[:4]
	heightInt := height[:4]
	ptyReq := fmt.Sprintf("TERM=%s; WIDTH=%d; HEIGHT=%d", termEnv, widthInt, heightInt)
	log.Printf("PTY request: %s\n", ptyReq)

	req.Reply(true, nil)
}

func handleEnv(channel ssh.Channel, req *ssh.Request) {
	req.Reply(true, nil)
}

func handleWindowChange(channel ssh.Channel, req *ssh.Request) {
	req.Reply(true, nil)
}
