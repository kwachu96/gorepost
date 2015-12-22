// Copyright 2015 Robert S. Gerus. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package bot

import (
	"encoding/json"
	"fmt"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/arachnist/gorepost/irc"
)

var trimTitle = regexp.MustCompile("[\\s]+")
var trimLink = regexp.MustCompile("^.*?http")
var enc = charmap.ISO8859_2

func youtube(vid string) string {
	var dat map[string]interface{}
	link := fmt.Sprintf("https://www.youtube.com/oembed?format=json&url=http://www.youtube.com/watch?v=%+v", vid)
	data, err := httpGet(link)
	if err != nil {
		return "error getting data from youtube"
	}

	err = json.Unmarshal(data, &dat)
	if err != nil {
		return "error decoding data from youtube"
	}
	return dat["title"].(string)
}

func youtubeLong(l string) string {
	pattern := regexp.MustCompile(`/watch[?]v[=](?P<vid>[a-zA-Z0-9-_]+)`)
	res := []byte{}
	for _, s := range pattern.FindAllSubmatchIndex([]byte(l), -1) {
		res = pattern.ExpandString(res, "$vid", l, s)
	}
	return youtube(string(res))
}

func youtubeShort(l string) string {
	pattern := regexp.MustCompile(`youtu.be/(?P<vid>[a-zA-Z0-9-_]+)`)
	res := []byte{}
	for _, s := range pattern.FindAllSubmatchIndex([]byte(l), -1) {
		res = pattern.ExpandString(res, "$vid", l, s)
	}
	return youtube(string(res))
}

func genericURLTitle(l string) string {
	title, err := httpGetXpath(l, "//head/title")
	if err == errElementNotFound {
		return "no title"
	} else if err != nil {
		return fmt.Sprint("error:", err)
	}

	title = string(trimTitle.ReplaceAll([]byte(title), []byte{' '})[:])
	if !utf8.ValidString(title) {
		title, _, err = transform.String(enc.NewDecoder(), title)
		if err != nil {
			return fmt.Sprint("error:", err)
		}
	}

	return title
}

var customDataFetchers = []struct {
	re      *regexp.Regexp
	fetcher func(l string) string
}{
	{
		re:      regexp.MustCompile("//(www.)?youtube.com/"),
		fetcher: youtubeLong,
	},
	{
		re:      regexp.MustCompile("//youtu.be/"),
		fetcher: youtubeShort,
	},
	{
		re:      regexp.MustCompile(".*"),
		fetcher: genericURLTitle,
	},
}

func linktitle(output func(irc.Message), msg irc.Message) {
	var r []string

	for _, s := range strings.Split(strings.Trim(msg.Trailing, "\001"), " ") {
		if s == "notitle" {
			return
		}

		b, _ := regexp.Match("https?://", []byte(s))

		s = string(trimLink.ReplaceAll([]byte(s), []byte("http"))[:])

		if b {
		FetchersLoop:
			for _, d := range customDataFetchers {
				if d.re.MatchString(s) {
					t := d.fetcher(s)
					if t != "no title" {
						r = append(r, t)
					}
					break FetchersLoop
				}
			}
		}
	}

	if len(r) > 0 {
		t := cfg.LookupString(msg.Context, "LinkTitlePrefix") + strings.Join(r, cfg.LookupString(msg.Context, "LinkTitleDelimiter"))

		output(reply(msg, t))
	}
}

func init() {
	addCallback("PRIVMSG", "LINKTITLE", linktitle)
}
