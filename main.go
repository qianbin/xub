package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type h func(w http.ResponseWriter, req *http.Request) error

func (fn h) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if err := fn(w, req); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type message struct {
	data        []byte
	contentType string
}

type slot struct {
	msg *message
	ack *message
}

type slots struct {
	slots map[string]*slot
	lock  sync.Mutex
}

func newSlots() *slots {
	return &slots{
		slots: make(map[string]*slot),
	}
}

func (s *slots) NewMsg(m *message) (id string) {
	id = uuid.Must(uuid.NewRandom()).String()

	s.lock.Lock()
	defer s.lock.Unlock()
	s.slots[id] = &slot{msg: m}
	return
}

func (s *slots) GetMsg(id string) *message {
	s.lock.Lock()
	defer s.lock.Unlock()

	slot := s.slots[id]
	if slot != nil {
		return slot.msg
	}
	return nil
}

func (s *slots) Ack(id string, ack *message) bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	slot := s.slots[id]
	if slot == nil {
		return false
	}
	slot.ack = ack
	return true
}

func (s *slots) GetAck(id string) *message {
	s.lock.Lock()
	defer s.lock.Unlock()
	slot := s.slots[id]
	if slot == nil {
		return nil
	}
	return slot.ack
}

func main() {

	slots := newSlots()

	router := mux.NewRouter()
	router.Path("/m").
		Methods(http.MethodPost).
		Handler(h(func(w http.ResponseWriter, req *http.Request) error {
			data, err := ioutil.ReadAll(req.Body)
			if err != nil {
				return err
			}

			id := slots.NewMsg(&message{data, req.Header.Get("content-type")})
			return json.NewEncoder(w).Encode(struct {
				ID string `json:"id"`
			}{id})
		}))
	router.Path("/m/{id}").
		Methods(http.MethodGet).
		Handler(h(func(w http.ResponseWriter, req *http.Request) error {
			id := mux.Vars(req)["id"]
			m := slots.GetMsg(id)
			if m != nil {
				w.Header().Set("Content-Type", m.contentType)
				w.Write(m.data)
			} else {
				http.NotFound(w, req)
			}
			return nil
		}))

	router.Path("/m/{id}/ack").
		Methods(http.MethodPost).
		Handler(h(func(w http.ResponseWriter, req *http.Request) error {
			id := mux.Vars(req)["id"]
			data, err := ioutil.ReadAll(req.Body)
			if err != nil {
				return err
			}
			if !slots.Ack(id, &message{data, req.Header.Get("content-type")}) {
				http.NotFound(w, req)
				return nil
			}
			return json.NewEncoder(w).Encode(struct{}{})
		}))

	router.Path("/m/{id}/ack").
		Methods(http.MethodGet).
		Handler(h(func(w http.ResponseWriter, req *http.Request) error {
			id := mux.Vars(req)["id"]

			ack := slots.GetAck(id)
			if ack == nil {
				http.NotFound(w, req)
				return nil
			}
			w.Header().Set("Content-Type", ack.contentType)
			w.Write(ack.data)
			return nil
		}))

	http.ListenAndServe(":8765", router)
}
