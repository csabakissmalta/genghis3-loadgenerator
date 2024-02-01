package processers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"

	"github.com/csabakissmalta/tpee/task"
)

var Counter int = 0

type SportsNavTree struct {
	GroupID    any        `json:"groupId"`
	Icon       any        `json:"icon"`
	Title      any        `json:"title"`
	TitleKey   any        `json:"titleKey"`
	ChildIndex int        `json:"childIndex"`
	Meta       Meta       `json:"meta"`
	Count      int        `json:"count"`
	Children   []Children `json:"children"`
}

type Meta struct {
	Outrights int `json:"outrights"`
	Events    int `json:"events"`
	Live      int `json:"live"`
	Today     int `json:"today"`
	ThreeHrs  int `json:"3hrs"`
	Four8Hrs  int `json:"48hrs"`
}

type Children struct {
	GroupID    string     `json:"groupId"`
	SportID    string     `json:"sportId,omitempty"`
	Icon       string     `json:"icon"`
	Title      string     `json:"title"`
	TitleKey   any        `json:"titleKey"`
	ChildIndex int        `json:"childIndex"`
	Meta       Meta       `json:"meta"`
	Count      int        `json:"count"`
	Children   []Children `json:"children"`
}

type BetbuilderTestMsgProcesser struct {
	// Data processing channel
	Schema any
}

func (bbp BetbuilderTestMsgProcesser) Process(raw_msg any) ([]byte, error) {
	// TODO: Implement parsing and serializing the data
	// into messages bytes and return it
	// The schema is the JSON model of what is sent
	Counter += 1

	log.Println("MSG is being processed...")
	resp, err := io.ReadAll(raw_msg.(*task.Task).Response.Body)
	if err != nil {
		log.Println("Could not read the response")
	}
	raw_msg.(*task.Task).Response.Body.Close()

	navtree := &SportsNavTree{}
	err = json.Unmarshal(resp, navtree)
	if err != nil {
		log.Println("ERR: Could not parse JOSON response")
	}

	p := fmt.Sprintf("%s%d", navtree.Children[0].Children[0].Children[0].Title, Counter)

	return []byte(p), nil
	// return []byte("CRAP"), nil
}
