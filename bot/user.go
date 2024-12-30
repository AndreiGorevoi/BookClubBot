package bot

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"sync"
)

const DBPath = "./db/db.json"

var mu sync.Mutex

type Subscriber struct {
	Id        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Nick      string `json:"nick"`
}

func persistSubs(subs []Subscriber) error {
	mu.Lock()
	defer mu.Unlock()
	f, err := os.Create(DBPath)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(subs)
	if err != nil {
		return err
	}

	_, err = f.Write(data)

	return err
}

func loadSubs() []Subscriber {
	res := make([]Subscriber, 0)
	f, err := os.Open(DBPath)
	if err != nil {
		log.Printf("cannot open dbpath: %s\n", DBPath)
		return res
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		log.Printf("cannot read from dbpath: %s\n", DBPath)
		return res
	}

	if len(data) == 0 {
		return res
	}

	err = json.Unmarshal(data, &res)
	if err != nil {
		log.Printf("cannot unmarshal data from dbpath: %s\n", DBPath)
	}
	return res
}
