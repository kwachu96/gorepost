package irc

import (
	"bufio"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"
)

const delim byte = '\n'
const endline string = "\r\n"

type Connection struct {
	Network   string
	Nick      string
	User      string
	RealName  string
	Input     chan Message
	Output    chan Message
	reader    *bufio.Reader
	writer    *bufio.Writer
	conn      net.Conn
	reconnect chan struct{}
	Quit      chan struct{}
	quitsend  chan struct{}
	quitrecv  chan struct{}
	l         sync.Mutex
}

func (c *Connection) Sender() {
	log.Println(c.Network, "spawned Sender")
	for {
		select {
		case msg := <-c.Input:
			c.writer.WriteString(msg.String() + endline)
			log.Println(c.Network, "-->", msg.String())
			c.writer.Flush()
		case <-c.quitsend:
			log.Println(c.Network, "closing Sender")
			return
		}
	}
}

func (c *Connection) Receiver() {
	log.Println(c.Network, "spawned Receiver")
	for {
		raw, err := c.reader.ReadString(delim)
		if err != nil {
			log.Println(c.Network, "error reading message", err.Error())
			log.Println(c.Network, "closing Receiver")
			c.Quit <- struct{}{}
			log.Println(c.Network, "sent quit message from Receiver")
			return
		}
		msg, err := ParseMessage(raw)
		if err != nil {
			log.Println(c.Network, "error decoding message", err.Error())
			log.Println(c.Network, "closing Receiver")
			c.Quit <- struct{}{}
			log.Println(c.Network, "sent quit message from Receiver")
			return
		} else {
			log.Println(c.Network, "<--", msg.String())
		}
		select {
		case c.Output <- *msg:
		case <-c.quitrecv:
			log.Println(c.Network, "closing Receiver")
			return
		}
	}
}

func (c *Connection) Cleaner() {
	log.Println(c.Network, "spawned Cleaner")
	for {
		<-c.Quit
		log.Println(c.Network, "ceceived quit message")
		c.l.Lock()
		log.Println(c.Network, "cleaning up!")
		c.quitsend <- struct{}{}
		c.quitrecv <- struct{}{}
		c.reconnect <- struct{}{}
		c.conn.Close()
		log.Println(c.Network, "closing Cleaner")
		c.l.Unlock()
	}
}

func (c *Connection) Keeper(servers []string) {
	log.Println(c.Network, "spawned Keeper")
	for {
		<-c.reconnect
		c.l.Lock()
		if c.Input != nil {
			close(c.Input)
			close(c.Output)
			close(c.quitsend)
			close(c.quitrecv)
		}
		c.Input = make(chan Message, 1)
		c.Output = make(chan Message, 1)
		c.quitsend = make(chan struct{}, 1)
		c.quitrecv = make(chan struct{}, 1)
		server := servers[rand.Intn(len(servers))]
		log.Println(c.Network, "connecting to", server)
		c.Dial(server)
		c.l.Unlock()

		go c.Sender()
		go c.Receiver()

		log.Println(c.Network, "Initializing IRC connection")
		c.Input <- Message{
			Command:  "NICK",
			Trailing: c.Nick,
		}
		c.Input <- Message{
			Command:  "USER",
			Params:   []string{c.User, "0", "*"},
			Trailing: c.RealName,
		}

	}
}

func (c *Connection) Setup(network string, servers []string, nick string, user string, realname string) {
	rand.Seed(time.Now().UnixNano())

	c.reconnect = make(chan struct{}, 1)
	c.Quit = make(chan struct{}, 1)
	c.Nick = nick
	c.User = user
	c.RealName = realname
	c.Network = network

	c.reconnect <- struct{}{}
	go c.Keeper(servers)
	go c.Cleaner()
	return
}

func (c *Connection) Dial(server string) error {

	conn, err := net.Dial("tcp", server)
	if err != nil {
		log.Println(c.Network, "Cannot connect to", server, "error:", err.Error())
		return err
	}
	log.Println(c.Network, "Connected to", server)
	c.writer = bufio.NewWriter(conn)
	c.reader = bufio.NewReader(conn)
	c.conn = conn

	return nil
}
