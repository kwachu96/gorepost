// +build go1.4

// Copyright 2015 Robert S. Gerus. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package bot

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/arachnist/dyncfg"
	"github.com/arachnist/gorepost/irc"
)

var eventTests = []struct {
	desc        string
	in          irc.Message
	expectedOut []irc.Message
}{
	{
		desc: "seen noone",
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":seen noone",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		expectedOut: []irc.Message{
			{
				Command:  "PRIVMSG",
				Params:   []string{"#testchan-1"},
				Trailing: "nope, never",
			},
		},
	},
	{
		desc: "ping",
		in: irc.Message{
			Command:  "PING",
			Trailing: "foobar",
		},
		expectedOut: []irc.Message{
			{
				Command:  "PONG",
				Trailing: "foobar",
			},
		},
	},
	{
		desc: "invitki",
		in: irc.Message{
			Command:  "INVITE",
			Trailing: "#test-channel",
		},
		expectedOut: []irc.Message{
			{
				Command: "JOIN",
				Params:  []string{"#test-channel"},
			},
		},
	},
	{
		desc: "channel join",
		in: irc.Message{
			Command: "001",
			Context: map[string]string{
				"Network": "TestNetwork",
			},
		},
		expectedOut: []irc.Message{
			{
				Command: "JOIN",
				Params:  []string{"#testchan-1"},
			},
			{
				Command: "JOIN",
				Params:  []string{"#testchan-2"},
			},
		},
	},
	{
		desc: "msgping",
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":ping",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		expectedOut: []irc.Message{
			{
				Command:  "PRIVMSG",
				Params:   []string{"#testchan-1"},
				Trailing: "pingity pong",
			},
		},
	},
	{
		desc: "nickserv",
		in: irc.Message{
			Command:  "NOTICE",
			Params:   []string{"gorepost"},
			Trailing: "This nickname is registered. Please choose a different nickname, or identify via …",
			Prefix: &irc.Prefix{
				Name: "NickServ",
				User: "NickServ",
				Host: "services.",
			},
		},
		expectedOut: []irc.Message{
			{
				Command:  "PRIVMSG",
				Params:   []string{"NickServ"},
				Trailing: "IDENTIFY test_password",
			},
		},
	},
	{
		desc: "pick",
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":pick test",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		expectedOut: []irc.Message{
			{
				Command:  "PRIVMSG",
				Params:   []string{"#testchan-1"},
				Trailing: "test",
			},
		},
	},
	{
		desc: "google",
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":g google.com",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		expectedOut: []irc.Message{
			{
				Command:  "PRIVMSG",
				Params:   []string{"#testchan-1"},
				Trailing: "Google https://www.google.com/",
			},
		},
	},
	{
		desc: "linktitle",
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: "https://www.google.com/",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		expectedOut: []irc.Message{
			{
				Command:  "PRIVMSG",
				Params:   []string{"#testchan-1"},
				Trailing: "↳ title: Google",
			},
		},
	},
	{
		desc: "nickserv channeljoin",
		in: irc.Message{
			Command:  "NOTICE",
			Params:   []string{"gorepost"},
			Trailing: "You are now identified",
			Prefix: &irc.Prefix{
				Name: "NickServ",
				User: "NickServ",
				Host: "services.",
			},
		},
		expectedOut: []irc.Message{
			{
				Command: "JOIN",
				Params:  []string{"#securechan-1"},
			},
			{
				Command: "JOIN",
				Params:  []string{"#securechan-2"},
			},
		},
	},
	{
		desc: "linktitle connection refused",
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: "http://127.0.0.1:333/conn-refused",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		expectedOut: []irc.Message{
			{
				Command:  "PRIVMSG",
				Params:   []string{"#testchan-1"},
				Trailing: "↳ title: error:Get http://127.0.0.1:333/conn-refused: dial tcp 127.0.0.1:333: getsockopt: connection refused",
			},
		},
	},
	{
		desc: "linktitle iso8859-2",
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: "http://arachnist.is-a.cat/test-iso8859-2.html",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		expectedOut: []irc.Message{
			{
				Command:  "PRIVMSG",
				Params:   []string{"#testchan-1"},
				Trailing: "↳ title: Tytuł używający przestarzałego kodowania znaków",
			},
		},
	},
	{
		desc: "linktitle common exploit",
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: "http://arachnist.is-a.cat/test.html",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		expectedOut: []irc.Message{
			{
				Command:  "PRIVMSG",
				Params:   []string{"#testchan-1"},
				Trailing: "↳ title: Tak Aż zbyt dobrze. Naprawdę QUIT dupa",
			},
		},
	},
	{
		desc: "papież",
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":papież",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
				User: "test",
				Host: "framework",
			},
		},
		expectedOut: []irc.Message{
			{
				Command:  "PRIVMSG",
				Params:   []string{"#testchan-1"},
				Trailing: "Papież adjective",
			},
		},
	},
	{
		desc: "jan without args",
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":jan",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
				User: "test",
				Host: "framework",
			},
		},
		expectedOut: []irc.Message{
			{
				Command:  "PRIVMSG",
				Params:   []string{"#testchan-1"},
				Trailing: "Jan Paweł II predicate małe object",
			},
		},
	},
	{
		desc: "jan predicate",
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":jan predicate-ł",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
				User: "test",
				Host: "framework",
			},
		},
		expectedOut: []irc.Message{
			{
				Command:  "PRIVMSG",
				Params:   []string{"#testchan-1"},
				Trailing: "Jan Paweł II predicate-ł małe object",
			},
		},
	},
	{
		desc: "jan object",
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":jan specified-object",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
				User: "test",
				Host: "framework",
			},
		},
		expectedOut: []irc.Message{
			{
				Command:  "PRIVMSG",
				Params:   []string{"#testchan-1"},
				Trailing: "Jan Paweł II predicate małe specified-object",
			},
		},
	},
	{
		desc: "roll multiple",
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":roll 1 3",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		expectedOut: []irc.Message{
			{
				Command:  "PRIVMSG",
				Params:   []string{"#testchan-1"},
				Trailing: "3",
			},
		},
	},
	{
		desc: "roll single",
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":roll 1",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		expectedOut: []irc.Message{
			{
				Command:  "PRIVMSG",
				Params:   []string{"#testchan-1"},
				Trailing: "1",
			},
		},
	},
	{
		desc: "roll too many arguments",
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":roll 0 1 2",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		expectedOut: []irc.Message{
			{
				Command:  "PRIVMSG",
				Params:   []string{"#testchan-1"},
				Trailing: "Usage: :roll <sides int> <rolls int>, each roll is [0, n)+1, size has to be >0",
			},
		},
	},
	{
		desc: "roll multiple strings",
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":roll string another",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		expectedOut: []irc.Message{
			{
				Command:  "PRIVMSG",
				Params:   []string{"#testchan-1"},
				Trailing: "Usage: :roll <sides int> <rolls int>, each roll is [0, n)+1, size has to be >0",
			},
		},
	},
	{
		desc: "roll string",
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":roll string",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		expectedOut: []irc.Message{
			{
				Command:  "PRIVMSG",
				Params:   []string{"#testchan-1"},
				Trailing: "Usage: :roll <sides int> <rolls int>, each roll is [0, n)+1, size has to be >0",
			},
		},
	},
	{
		desc: "roll 1000000",
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":roll 2000000",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		expectedOut: []irc.Message{
			{
				Command:  "PRIVMSG",
				Params:   []string{"#testchan-1"},
				Trailing: "Number of rolls and dice size is limited to 1000000",
			},
		},
	},
	{
		desc: "roll 0",
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":roll 0",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		expectedOut: []irc.Message{
			{
				Command:  "PRIVMSG",
				Params:   []string{"#testchan-1"},
				Trailing: "Usage: :roll <sides int> <rolls int>, each roll is [0, n)+1, size has to be >0",
			},
		},
	},
}

func TestPlugins(t *testing.T) {
	var r []irc.Message
	var m sync.Mutex
	var wg sync.WaitGroup

	// fake irc.Conn Sender replacement
	output := func(msg irc.Message) {
		m.Lock()
		defer m.Unlock()
		wg.Done()
		r = append(r, msg)
	}

	for _, e := range eventTests {
		t.Log("Running test", e.desc)
		r = r[:0]

		wg.Add(len(e.expectedOut))

		Dispatcher(output, e.in)

		time.Sleep(3000000 * time.Nanosecond)

		wg.Wait()

		m.Lock()
		if fmt.Sprintf("%+v", r) != fmt.Sprintf("%+v", e.expectedOut) {
			t.Logf("expected: %+v\n", e.expectedOut)
			t.Logf("result: %+v\n", r)
			t.Fail()
		}
		m.Unlock()
	}
}

var noResponseEvents = []struct {
	desc string
	in   irc.Message
}{
	{
		desc: "linktitle missing title",
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: "http://arachnist.is-a.cat/test-no-title.html",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
				User: "test",
				Host: "framework",
			},
		},
	},
	{
		desc: "linktitle notitle",
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: "https://www.google.com/ notitle",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
				User: "test",
				Host: "framework",
			},
		},
	},
	{
		desc: "nickserv spoof",
		in: irc.Message{
			Command:  "NOTICE",
			Params:   []string{"gorepost"},
			Trailing: "This nickname is registered. Please choose a different nickname, or identify via …",
			Prefix: &irc.Prefix{
				Name: "NickServ",
				User: "NickServ",
				Host: "fake.",
			},
		},
	},
	{
		desc: "nickserv other message",
		in: irc.Message{
			Command:  "NOTICE",
			Params:   []string{"gorepost"},
			Trailing: "Some other random message…",
			Prefix: &irc.Prefix{
				Name: "NickServ",
				User: "NickServ",
				Host: "services.",
			},
		},
	},
	{
		desc: "non-matching",
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: "foo bar baz",
			Prefix: &irc.Prefix{
				Name: "test-framework",
				User: "test",
				Host: "framework",
			},
		},
	},
	{
		desc: "seen without arguments",
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":seen",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
	},
}

func TestNoResponse(t *testing.T) {
	output := func(msg irc.Message) {
		t.Logf("Got a response: %+v\n", msg)
		t.Fail()
	}

	for _, e := range noResponseEvents {
		t.Log("Running test", e.desc)
		Dispatcher(output, e.in)

		time.Sleep(100000000 * time.Nanosecond) // 1*10^8 => 0.1 seconds
	}
}

var seenTestSeedEvents = []irc.Message{
	{
		Command:  "JOIN",
		Trailing: "",
		Params:   []string{"#testchan-1"},
		Prefix: &irc.Prefix{
			Name: "join",
			User: "seen",
			Host: "test",
		},
	},
	{
		Command:  "PRIVMSG",
		Trailing: "that's a text",
		Params:   []string{"#testchan-1"},
		Prefix: &irc.Prefix{
			Name: "privmsg",
			User: "seen",
			Host: "test",
		},
	},
	{
		Command:  "NOTICE",
		Trailing: "that's a notice",
		Params:   []string{"#testchan-1"},
		Prefix: &irc.Prefix{
			Name: "notice",
			User: "seen",
			Host: "test",
		},
	},
	{
		Command:  "PART",
		Trailing: "i'm leaving you",
		Params:   []string{"#testchan-1"},
		Prefix: &irc.Prefix{
			Name: "part",
			User: "seen",
			Host: "test",
		},
	},
	{
		Command:  "QUIT",
		Trailing: "that's a quit message",
		Params:   []string{},
		Prefix: &irc.Prefix{
			Name: "quit",
			User: "seen",
			Host: "test",
		},
	},
}

var seenTests = []struct {
	in       irc.Message
	outRegex string
}{
	{
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":seen join",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		outRegex: "^Last seen join on /#testchan-1 at .* joining$",
	},
	{
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":seen privmsg",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		outRegex: "^Last seen privmsg on /#testchan-1 at .* saying: that's a text$",
	},
	{
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":seen notice",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		outRegex: "^Last seen notice on /#testchan-1 at .* noticing: that's a notice$",
	},
	{
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":seen part",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		outRegex: "^Last seen part on /#testchan-1 at .* leaving: i'm leaving you$",
	},
	{
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":seen quit",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		outRegex: "^Last seen quit on / at .* quitting with reasson: that's a quit message$",
	},
}

func TestSeenConditions(t *testing.T) {
	var wg sync.WaitGroup

	failOutput := func(msg irc.Message) {
		t.Log("these should not output anything")
		t.Fail()
	}

	genOutTestFunction := func(regex string) func(irc.Message) {
		return func(m irc.Message) {
			t.Logf("testing regex %+v on %+v", regex, m.Trailing)
			if b, _ := regexp.Match(regex, []byte(m.Trailing)); !b {
				t.Log("Failed", m.Trailing)
				t.Fail()
			}
			wg.Done()
		}
	}

	wg.Add(len(seenTestSeedEvents))
	for _, e := range seenTestSeedEvents {
		t.Logf("Filling seen db with: %+v\n", e)
		seenrecord(failOutput, e)
		wg.Done()
	}
	wg.Wait()

	wg.Add(len(seenTests))
	for _, e := range seenTests {
		seen(genOutTestFunction(e.outRegex), e.in)
	}

	wg.Wait()
}

var variableOutputTestEvents = []struct {
	in       irc.Message
	outRegex regexp.Regexp
	function func(func(irc.Message), irc.Message)
}{
	{
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":kotki",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		outRegex: *regexp.MustCompile("^http://.*tumblr"),
		function: kotki,
	},
	{
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":cycki",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		outRegex: *regexp.MustCompile("^cycki [(]nsfw[)]: http://.*"),
		function: cycki,
	},
	{
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":bonjour",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		outRegex: *regexp.MustCompile("^bonjour [(]nsfw[)]: http://.*"),
		function: bonjour,
	},
	{
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":korwin",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		outRegex: *regexp.MustCompile("^."),
		function: korwin,
	},
	{
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: ":at",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		outRegex: *regexp.MustCompile("^at:"),
		function: at,
	},
	{
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		outRegex: *regexp.MustCompile("^↳ title: Rick Astley - Never Gonna Give You Up by RickAstleyVEVO$"),
		function: linktitle,
	},
	{
		in: irc.Message{
			Command:  "PRIVMSG",
			Trailing: "http://youtu.be/dQw4w9WgXcQ",
			Params:   []string{"#testchan-1"},
			Prefix: &irc.Prefix{
				Name: "idontexist",
			},
		},
		outRegex: *regexp.MustCompile("^↳ title: Rick Astley - Never Gonna Give You Up by RickAstleyVEVO$"),
		function: linktitle,
	},
}

func TestFunctionsWithVariableOutput(t *testing.T) {
	var wg sync.WaitGroup

	genOutTestFunction := func(regex regexp.Regexp) func(irc.Message) {
		return func(m irc.Message) {
			t.Logf("testing regex /%v/ on %+v", regex.String(), m.Trailing)
			if !regex.Match([]byte(m.Trailing)) {
				t.Log("Failed", m.Trailing)
				t.Fail()
			}
			wg.Done()
		}
	}

	wg.Add(len(variableOutputTestEvents))
	for _, e := range variableOutputTestEvents {
		e.function(genOutTestFunction(e.outRegex), e.in)
	}

	wg.Wait()
}

func configLookupHelper(map[string]string) []string {
	return []string{".testconfig.json"}
}

func TestMain(m *testing.M) {
	log.SetOutput(ioutil.Discard)
	os.Exit(m.Run())
}

func init() {
	Initialize(dyncfg.New(configLookupHelper))
}
