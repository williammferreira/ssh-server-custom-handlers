package main

import (
	"golang.org/x/crypto/ssh"
	"log"
	"net"
	"os"
)

func main() {
	config := &ssh.ServerConfig{
		NoClientAuth: true,
	}

	privateBytes, err := os.ReadFile("id_rsa")
	if err != nil {
		log.Fatal("Failed to load private key")
	}

	private, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil {
		log.Fatal("Failed to parse private key")
	}

	config.AddHostKey(private)

	listener, err := net.Listen("tcp", "127.0.0.1:2222")
	if err != nil {
		log.Fatalf("Failed to listen on 2222: %s", err)
	}

	for {
		nConn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept incoming connection: %s", err)
			continue
		}

		go handleConnection(nConn, config)
	}
}
