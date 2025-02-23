package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"

	"github.com/reiver/go-oi"
	"github.com/reiver/go-telnet"
	"github.com/reiver/go-telnet/telsh"
)

type TelnetChatServer struct {
	rooms         map[string][]io.WriteCloser
	userRooms     map[io.WriteCloser]string
	handler       *telsh.ShellHandler
	roomsLock     sync.Mutex
	userRoomsLock sync.Mutex
}

func (cs *TelnetChatServer) JoinHandlerFunc(stdin io.ReadCloser, stdout io.WriteCloser, stderr io.WriteCloser, args ...string) error {
	if len(args) < 1 {
		stdout.Write([]byte("Usage: join <room number>\r\n"))
		return nil
	}

	room := args[0]

	cs.roomsLock.Lock()
	defer cs.roomsLock.Unlock()

	if arr, ok := cs.rooms[room]; ok {
		cs.rooms[room] = append(arr, stdout)
		cs.userRooms[stdout] = room
		return nil
	}

	cs.rooms[room] = []io.WriteCloser{stdout}
	cs.userRooms[stdout] = room

	msg := fmt.Sprintf("Joined room %s\r\n", room)
	stdout.Write([]byte(msg))

	return nil
}

func (cs *TelnetChatServer) JoinProducerFunc(ctx telnet.Context, name string, args ...string) telsh.Handler {
	return telsh.PromoteHandlerFunc(cs.JoinHandlerFunc, args...)
}

func (cs *TelnetChatServer) LeaveHandlerFunc(stdin io.ReadCloser, stdout io.WriteCloser, stderr io.WriteCloser, args ...string) error {
	stdout.Write([]byte("Leave Handler Called for room "))

	cs.userRoomsLock.Lock()
	cs.roomsLock.Lock()

	defer cs.roomsLock.Unlock()
	defer cs.userRoomsLock.Unlock()

	room, ok := cs.userRooms[stdout]
	if !ok {
		return nil
	}

	for i := 0; i < len(cs.rooms[room]); i++ {
		if cs.rooms[room][i] == stdout {
			stdout.Write([]byte("Left room " + room))
			cs.rooms[room] = append(cs.rooms[room][0:i], cs.rooms[room][i+1:]...)
			break
		}
	}

	return nil
}

func (cs *TelnetChatServer) LeaveProducerFunc(ctx telnet.Context, name string, args ...string) telsh.Handler {
	return telsh.PromoteHandlerFunc(cs.LeaveHandlerFunc, args...)
}

func (cs *TelnetChatServer) SendHandlerFunc(stdin io.ReadCloser, stdout io.WriteCloser, stderr io.WriteCloser, args ...string) error {
	cs.userRoomsLock.Lock()
	cs.roomsLock.Lock()

	defer cs.userRoomsLock.Unlock()
	defer cs.roomsLock.Unlock()

	room := cs.userRooms[stdout]

	for _, writer := range cs.rooms[room] {
		if writer != stdout {
			writer.Write([]byte(strings.Join(args, "")))
		}
	}
	return nil
}

func (cs *TelnetChatServer) SendProducerFunc(ctx telnet.Context, name string, args ...string) telsh.Handler {
	return telsh.PromoteHandlerFunc(cs.SendHandlerFunc, args...)
}

func (cs *TelnetChatServer) Serve(port string) error {
	return telnet.ListenAndServe(port, cs.handler)
}

func NewChatServer() *TelnetChatServer {
	server := &TelnetChatServer{
		rooms:     map[string][]io.WriteCloser{},
		userRooms: map[io.WriteCloser]string{},
	}

	shellHandler := telsh.NewShellHandler()
	shellHandler.ExitMessage = "Good Byte!"
	shellHandler.Prompt = "\n> "

	// Join
	commandJoin := "join"
	joinProducer := telsh.ProducerFunc(server.JoinProducerFunc)
	shellHandler.Register(commandJoin, joinProducer)

	// Leave
	commandLeave := "leave"
	leaveProducer := telsh.ProducerFunc(server.LeaveProducerFunc)
	shellHandler.Register(commandLeave, leaveProducer)

	// Send
	commandSend := "send"
	sendProducer := telsh.ProducerFunc(server.SendProducerFunc)
	shellHandler.Register(commandSend, sendProducer)

	server.handler = shellHandler
	return server
}

func main() {
	server := NewChatServer()
	if err := server.Serve(":5555"); err != nil {
		log.Fatalf("Error starting Telnet server: %v", err)
	}
}

type DiscordHandler struct{}

func (d DiscordHandler) ServeTELNET(ctx telnet.Context, w telnet.Writer, r telnet.Reader) {
	var acc bytes.Buffer
	tmp := make([]byte, 64)

	for {
		read, err := r.Read(tmp)
		if err != nil {
			break
		}

		if read > 0 {
			acc.Write(tmp[:read])
			// Check if the latest read chunk contains a newline
			if bytes.Contains(tmp[:read], []byte("\n")) {
				// Optionally trim newline and any surrounding whitespace
				message := strings.TrimSpace(acc.String())

				response := []byte("You wrote: " + message + "\n")
				if _, err := oi.LongWrite(w, response); err != nil {
					log.Printf("Error writing response: %v", err)
				}

				acc.Reset()
			}
		}
	}
}
