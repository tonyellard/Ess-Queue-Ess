// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/md5"
	"encoding/hex"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Message represents an SQS message
type Message struct {
	MessageID              string                 `json:"MessageId"`
	ReceiptHandle          string                 `json:"ReceiptHandle,omitempty"`
	MD5OfBody              string                 `json:"MD5OfBody"`
	Body                   string                 `json:"Body"`
	Attributes             map[string]string      `json:"Attributes,omitempty"`
	MessageAttributes      map[string]interface{} `json:"MessageAttributes,omitempty"`
	MD5OfMessageAttributes string                 `json:"MD5OfMessageAttributes,omitempty"`

	// Internal fields
	SentTimestamp     time.Time
	ReceiveCount      int
	FirstReceivedTime time.Time
	VisibilityTimeout time.Time
	DelayUntil        time.Time
}

// Queue represents an SQS queue
type Queue struct {
	Name       string
	URL        string
	Attributes map[string]string
	Messages   []*Message
	mu         sync.RWMutex

	// Queue configuration
	VisibilityTimeout      int // seconds
	MessageRetentionPeriod int // seconds
	MaximumMessageSize     int // bytes
	DelaySeconds           int
	ReceiveMessageWaitTime int // seconds (long polling)
}

// QueueManager manages all queues
type QueueManager struct {
	queues map[string]*Queue
	mu     sync.RWMutex
}

// NewQueueManager creates a new queue manager
func NewQueueManager() *QueueManager {
	return &QueueManager{
		queues: make(map[string]*Queue),
	}
}

// CreateQueue creates a new queue
func (qm *QueueManager) CreateQueue(name string, attributes map[string]string) (*Queue, error) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	if _, exists := qm.queues[name]; exists {
		return qm.queues[name], nil // Return existing queue
	}

	queue := &Queue{
		Name:                   name,
		URL:                    "/" + name,
		Attributes:             attributes,
		Messages:               make([]*Message, 0),
		VisibilityTimeout:      30,     // default 30 seconds
		MessageRetentionPeriod: 345600, // default 4 days
		MaximumMessageSize:     262144, // default 256 KB
		DelaySeconds:           0,
		ReceiveMessageWaitTime: 0,
	}

	qm.queues[name] = queue
	return queue, nil
}

// GetQueue retrieves a queue by name
func (qm *QueueManager) GetQueue(name string) (*Queue, bool) {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	queue, exists := qm.queues[name]
	return queue, exists
}

// DeleteQueue removes a queue
func (qm *QueueManager) DeleteQueue(name string) bool {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	if _, exists := qm.queues[name]; exists {
		delete(qm.queues, name)
		return true
	}
	return false
}

// ListQueues returns all queue URLs
func (qm *QueueManager) ListQueues(prefix string) []string {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	urls := make([]string, 0)
	for name, queue := range qm.queues {
		if prefix == "" || len(name) >= len(prefix) && name[:len(prefix)] == prefix {
			urls = append(urls, queue.URL)
		}
	}
	return urls
}

// GetAllQueues returns all queues (for admin UI)
func (qm *QueueManager) GetAllQueues() []*Queue {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	queues := make([]*Queue, 0, len(qm.queues))
	for _, queue := range qm.queues {
		queues = append(queues, queue)
	}
	return queues
}

// SendMessage adds a message to the queue
func (q *Queue) SendMessage(body string, attributes map[string]interface{}, delaySeconds int) *Message {
	q.mu.Lock()
	defer q.mu.Unlock()

	msg := &Message{
		MessageID:         uuid.New().String(),
		Body:              body,
		MD5OfBody:         calculateMD5(body),
		MessageAttributes: attributes,
		SentTimestamp:     time.Now(),
		ReceiveCount:      0,
		DelayUntil:        time.Now().Add(time.Duration(delaySeconds) * time.Second),
	}

	q.Messages = append(q.Messages, msg)
	return msg
}

// ReceiveMessages retrieves messages from the queue
func (q *Queue) ReceiveMessages(maxMessages int, visibilityTimeout int, waitTimeSeconds int) []*Message {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	available := make([]*Message, 0)

	for _, msg := range q.Messages {
		// Check if message is available (not delayed, not invisible)
		if now.After(msg.DelayUntil) && now.After(msg.VisibilityTimeout) {
			available = append(available, msg)
			if len(available) >= maxMessages {
				break
			}
		}
	}

	// Mark messages as invisible and set receipt handles
	for _, msg := range available {
		msg.ReceiptHandle = uuid.New().String()
		msg.VisibilityTimeout = now.Add(time.Duration(visibilityTimeout) * time.Second)
		msg.ReceiveCount++
		if msg.ReceiveCount == 1 {
			msg.FirstReceivedTime = now
		}
	}

	return available
}

// DeleteMessage removes a message from the queue
func (q *Queue) DeleteMessage(receiptHandle string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i, msg := range q.Messages {
		if msg.ReceiptHandle == receiptHandle {
			// Remove message
			q.Messages = append(q.Messages[:i], q.Messages[i+1:]...)
			return true
		}
	}
	return false
}

// PurgeQueue removes all messages
func (q *Queue) PurgeQueue() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Messages = make([]*Message, 0)
}

// GetAttributes returns queue attributes
func (q *Queue) GetAttributes() map[string]string {
	q.mu.RLock()
	defer q.mu.RUnlock()

	now := time.Now()
	visibleCount := 0
	notVisibleCount := 0
	delayedCount := 0

	for _, msg := range q.Messages {
		if now.Before(msg.DelayUntil) {
			delayedCount++
		} else if now.Before(msg.VisibilityTimeout) {
			notVisibleCount++
		} else {
			visibleCount++
		}
	}

	attrs := make(map[string]string)
	attrs["ApproximateNumberOfMessages"] = strconv.Itoa(visibleCount)
	attrs["ApproximateNumberOfMessagesNotVisible"] = strconv.Itoa(notVisibleCount)
	attrs["ApproximateNumberOfMessagesDelayed"] = strconv.Itoa(delayedCount)
	attrs["QueueArn"] = "arn:aws:sqs:us-east-1:000000000000:" + q.Name

	return attrs
}

// Helper functions
func calculateMD5(s string) string {
	hash := md5.Sum([]byte(s))
	return hex.EncodeToString(hash[:])
}
