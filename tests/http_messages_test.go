package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"fergus.molloy.xyz/vfmp/internal/model"
)

var topicCounter = atomic.Int64{}

func getTopicName() string {
	var alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

	result := make([]byte, 4)
	num := topicCounter.Add(1) - 1

	// Convert counter to base-26 representation (AAAA to ZZZZ)
	for i := 3; i >= 0; i-- {
		result[i] = alphabet[num%26]
		num /= 26
	}

	return string(result)
}

func TestCreateMessage(t *testing.T) {
	cfg := getTestConfig(t)
	topic := getTopicName()

	reader := bytes.NewBufferString("test-message")
	r, err := http.Post(fmt.Sprintf("http://%s/messages/%s", cfg.HTTPAddr, topic), "text/plain", reader)

	if err != nil {
		t.Fatalf("failed to send request: %v", err)
	}
	if r.StatusCode != http.StatusOK {
		t.Fatalf("request was unsuccessful, got code: %d expected: %d", r.StatusCode, http.StatusOK)
	}
}

func CreateMessage(t *testing.T, addr string) string {
	t.Helper()
	topic := getTopicName()

	reader := bytes.NewBufferString("test-message")
	r, err := http.Post(fmt.Sprintf("http://%s/messages/%s", addr, topic), "text/plain", reader)

	if err != nil {
		t.Fatalf("failed to send request: %v", err)
	}
	if r.StatusCode != http.StatusOK {
		t.Fatalf("request was unsuccessful, got code: %d expected: %d", r.StatusCode, http.StatusOK)
	}
	return topic
}

func SendGet(t *testing.T, url string, status int) *http.Response {
	t.Helper()

	r, err := http.Get(url)
	if err != nil {
		t.Fatalf("failed to send request: %v", err)
	}
	if r.StatusCode != status {
		t.Fatalf("request was unsuccessful, got code: %d expected: %d", r.StatusCode, status)
	}
	return r
}

type CountResponse struct {
	Count int `json:"count"`
}

func TestCheckQueueLength(t *testing.T) {
	cfg := getTestConfig(t)
	topic := CreateMessage(t, cfg.HTTPAddr)

	r := SendGet(t, fmt.Sprintf("http://%s/messages/%s?data=count", cfg.HTTPAddr, topic), 200)
	defer r.Body.Close()

	var j CountResponse
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("could not ready response body: %v", err)
	}
	err = json.Unmarshal(body, &j)
	if err != nil {
		t.Fatalf("could not unmarshal response body: %v", err)
	}

	if j.Count != 1 {
		t.Fatalf("topic had wrong message count, expected: %d, got: %d", 1, j.Count)
	}
}

func TestPeekMessage(t *testing.T) {
	cfg := getTestConfig(t)
	topic := CreateMessage(t, cfg.HTTPAddr)

	r := SendGet(t, fmt.Sprintf("http://%s/messages/%s?data=peek", cfg.HTTPAddr, topic), 200)
	defer r.Body.Close()

	var j model.Message
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("could not ready response body: %v", err)
	}
	err = json.Unmarshal(body, &j)
	if err != nil {
		t.Fatalf("could not unmarshal response body: %v", err)
	}

	expected := []byte("test-message")
	if !slices.Equal(j.Data, expected) {
		t.Fatalf("peeked message had wrong data, expected: %v, got: %v", expected, j.Data)
	}
}

func TestCreateMessagesUnbuffered(t *testing.T) {
	cfg := getTestConfig(t)
	topic := getTopicName()

	for range 10 {
		client := &http.Client{
			Timeout: time.Second,
		}
		reader := bytes.NewBufferString("test-message")
		r, err := client.Post(fmt.Sprintf("http://%s/messages/%s", cfg.HTTPAddr, topic), "text/plain", reader)
		if err != nil {
			t.Fatalf("failed to send request: %v", err)
			t.FailNow()
		}
		if r.StatusCode != 200 {
			t.Fatalf("request was unsuccessful, got code: %d expected: %d", r.StatusCode, 200)
			t.FailNow()
		}
	}
}
