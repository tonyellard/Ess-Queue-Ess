// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/md5"
	"encoding/hex"
	"log"
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

	// FIFO-specific fields
	MessageDeduplicationId string `json:"MessageDeduplicationId,omitempty"`
	MessageGroupId         string `json:"MessageGroupId,omitempty"`
	SequenceNumber         string `json:"SequenceNumber,omitempty"`

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
	MaxReceiveCount        int // maximum receive count before DLQ (if configured)

	// FIFO configuration
	FifoQueue                 bool
	ContentBasedDeduplication bool
	deduplicationCache        map[string]time.Time // deduplicationId -> timestamp
	sequenceNumber            int64

	// DLQ configuration
	RedrivePolicy      *RedrivePolicy
	RedriveAllowPolicy *RedriveAllowPolicy

	// Background processing
	stopChan chan struct{}
}

// RedrivePolicy defines Dead Letter Queue configuration
type RedrivePolicy struct {
	DeadLetterTargetArn string `json:"deadLetterTargetArn"`
	MaxReceiveCount     int    `json:"maxReceiveCount"`
}

// RedriveAllowPolicy defines which queues can use this as a DLQ
type RedriveAllowPolicy struct {
	RedrivePermission string   `json:"redrivePermission"` // allowAll, denyAll, byQueue
	SourceQueueArns   []string `json:"sourceQueueArns,omitempty"`
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
		MaxReceiveCount:        3, // default max receive count
		deduplicationCache:     make(map[string]time.Time),
		sequenceNumber:         0,
		stopChan:               make(chan struct{}),
	}

	// Start background goroutine to check visibility timeouts and DLQ
	go queue.backgroundChecker()

	// Check if this is a FIFO queue (by name or by attribute)
	if len(name) > 5 && name[len(name)-5:] == ".fifo" {
		queue.FifoQueue = true
	}
	if fifoAttr, ok := attributes["FifoQueue"]; ok && fifoAttr == "true" {
		queue.FifoQueue = true
	}

	// Parse FIFO attributes
	if contentBased, ok := attributes["ContentBasedDeduplication"]; ok && contentBased == "true" {
		queue.ContentBasedDeduplication = true
	}

	// Parse MaxReceiveCount
	if maxReceiveStr, ok := attributes["MaxReceiveCount"]; ok {
		if maxReceive, err := strconv.Atoi(maxReceiveStr); err == nil && maxReceive > 0 {
			queue.MaxReceiveCount = maxReceive
		}
	}

	// Parse RedrivePolicy
	if redrivePolicyStr, ok := attributes["RedrivePolicy"]; ok {
		queue.RedrivePolicy = parseRedrivePolicy(redrivePolicyStr)
	}

	// Parse RedriveAllowPolicy
	if redriveAllowPolicyStr, ok := attributes["RedriveAllowPolicy"]; ok {
		queue.RedriveAllowPolicy = parseRedriveAllowPolicy(redriveAllowPolicyStr)
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
	if queue, exists := qm.queues[name]; exists {
		// Stop background checker
		close(queue.stopChan)
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
func (q *Queue) SendMessage(body string, attributes map[string]interface{}, delaySeconds int, deduplicationId, groupId string) *Message {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Handle FIFO deduplication
	if q.FifoQueue {
		// Determine deduplication ID
		if deduplicationId == "" && q.ContentBasedDeduplication {
			deduplicationId = calculateMD5(body)
		}

		// Check deduplication cache (5-minute window)
		if deduplicationId != "" {
			if lastSent, exists := q.deduplicationCache[deduplicationId]; exists {
				if time.Since(lastSent) < 5*time.Minute {
					// Find and return the existing message
					for _, msg := range q.Messages {
						if msg.MessageDeduplicationId == deduplicationId {
							return msg
						}
					}
				}
			}
			q.deduplicationCache[deduplicationId] = time.Now()
		}
	}

	q.sequenceNumber++
	sequenceNum := strconv.FormatInt(q.sequenceNumber, 10)

	msg := &Message{
		MessageID:              uuid.New().String(),
		Body:                   body,
		MD5OfBody:              calculateMD5(body),
		MessageAttributes:      attributes,
		SentTimestamp:          time.Now(),
		ReceiveCount:           0,
		DelayUntil:             time.Now().Add(time.Duration(delaySeconds) * time.Second),
		MessageDeduplicationId: deduplicationId,
		MessageGroupId:         groupId,
		SequenceNumber:         sequenceNum,
	}

	q.Messages = append(q.Messages, msg)
	return msg
}

// backgroundChecker runs every second to check for expired visibility timeouts and move messages to DLQ
func (q *Queue) backgroundChecker() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			q.checkVisibilityTimeoutsAndDLQ()
		case <-q.stopChan:
			return
		}
	}
}

// checkVisibilityTimeoutsAndDLQ checks for messages with expired visibility timeouts that should move to DLQ
func (q *Queue) checkVisibilityTimeoutsAndDLQ() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.RedrivePolicy == nil {
		return // No DLQ configured
	}

	now := time.Now()
	messagesToMove := make([]*Message, 0)

	for _, msg := range q.Messages {
		// Check if message is currently visible (visibility timeout has expired)
		if now.After(msg.VisibilityTimeout) && now.After(msg.DelayUntil) {
			// If message has been received MaxReceiveCount times or more, move to DLQ
			if msg.ReceiveCount >= q.RedrivePolicy.MaxReceiveCount {
				// Log for debugging
				log.Printf("[DLQ] Queue %s: Moving message %s to DLQ (ReceiveCount=%d, MaxReceiveCount=%d, VisibilityTimeout=%v, Now=%v)",
					q.Name, msg.MessageID, msg.ReceiveCount, q.RedrivePolicy.MaxReceiveCount, msg.VisibilityTimeout, now)
				messagesToMove = append(messagesToMove, msg)
			}
		}
	}

	// Move messages to DLQ
	for _, msg := range messagesToMove {
		q.moveToDLQ(msg)
	}
}

// ReceiveMessages retrieves messages from the queue
func (q *Queue) ReceiveMessages(maxMessages int, visibilityTimeout int, waitTimeSeconds int) []*Message {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	available := make([]*Message, 0)

	if q.FifoQueue {
		// For FIFO queues, group messages by MessageGroupId and return in order
		groupMap := make(map[string][]*Message)
		for _, msg := range q.Messages {
			if now.After(msg.DelayUntil) && now.After(msg.VisibilityTimeout) {
				groupId := msg.MessageGroupId
				if groupId == "" {
					groupId = "default"
				}
				groupMap[groupId] = append(groupMap[groupId], msg)
			}
		}

		// Return messages from each group in order, one message per group
		for _, msgs := range groupMap {
			if len(msgs) > 0 {
				available = append(available, msgs[0])
				if len(available) >= maxMessages {
					break
				}
			}
		}
	} else {
		// Standard queue: return messages in any order
		for _, msg := range q.Messages {
			if now.After(msg.DelayUntil) && now.After(msg.VisibilityTimeout) {
				available = append(available, msg)
				if len(available) >= maxMessages {
					break
				}
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
		log.Printf("[RECEIVE] Queue %s: Message %s received (ReceiveCount=%d, VisibilityTimeout set to %v, timeout param=%ds)",
			q.Name, msg.MessageID, msg.ReceiveCount, msg.VisibilityTimeout, visibilityTimeout)
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

// moveToDLQ moves a message to the dead letter queue
func (q *Queue) moveToDLQ(msg *Message) {
	if q.RedrivePolicy == nil {
		return
	}

	// Extract DLQ name from ARN
	dlqName := extractQueueNameFromArn(q.RedrivePolicy.DeadLetterTargetArn)

	dlq, exists := queueManager.GetQueue(dlqName)
	if !exists {
		return
	}

	// Remove from current queue
	for i, m := range q.Messages {
		if m.MessageID == msg.MessageID {
			q.Messages = append(q.Messages[:i], q.Messages[i+1:]...)
			break
		}
	}

	// Reset message state for DLQ
	msg.ReceiptHandle = ""
	msg.VisibilityTimeout = time.Time{}
	msg.DelayUntil = time.Now()

	// Add to DLQ
	dlq.mu.Lock()
	dlq.Messages = append(dlq.Messages, msg)
	dlq.mu.Unlock()
}

// RedriveMessages moves messages from this DLQ back to the source queue
func (qm *QueueManager) RedriveMessages(dlqName, sourceQueueArn string, maxMessages int) int {
	dlq, exists := qm.GetQueue(dlqName)
	if !exists {
		return 0
	}

	sourceQueueName := extractQueueNameFromArn(sourceQueueArn)
	sourceQueue, exists := qm.GetQueue(sourceQueueName)
	if !exists {
		return 0
	}

	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	movedCount := 0
	messagesToMove := make([]*Message, 0)

	for i, msg := range dlq.Messages {
		if maxMessages > 0 && movedCount >= maxMessages {
			break
		}
		messagesToMove = append(messagesToMove, msg)
		dlq.Messages = append(dlq.Messages[:i], dlq.Messages[i+1:]...)
		movedCount++
	}

	// Move messages to source queue
	sourceQueue.mu.Lock()
	for _, msg := range messagesToMove {
		msg.ReceiptHandle = ""
		msg.VisibilityTimeout = time.Time{}
		msg.ReceiveCount = 0
		msg.DelayUntil = time.Now()
		sourceQueue.Messages = append(sourceQueue.Messages, msg)
	}
	sourceQueue.mu.Unlock()

	return movedCount
}

// Helper functions
func calculateMD5(s string) string {
	hash := md5.Sum([]byte(s))
	return hex.EncodeToString(hash[:])
}

func parseRedrivePolicy(policyJSON string) *RedrivePolicy {
	// Simple JSON parsing for RedrivePolicy
	// Format: {"deadLetterTargetArn":"arn:aws:sqs:us-east-1:000000000000:my-dlq","maxReceiveCount":3}
	policy := &RedrivePolicy{}

	// Extract deadLetterTargetArn
	if start := findJSONValue(policyJSON, "deadLetterTargetArn"); start != "" {
		policy.DeadLetterTargetArn = start
	}

	// Extract maxReceiveCount
	if countStr := findJSONValue(policyJSON, "maxReceiveCount"); countStr != "" {
		if count, err := strconv.Atoi(countStr); err == nil {
			policy.MaxReceiveCount = count
		}
	}

	return policy
}

func parseRedriveAllowPolicy(policyJSON string) *RedriveAllowPolicy {
	policy := &RedriveAllowPolicy{}

	if permission := findJSONValue(policyJSON, "redrivePermission"); permission != "" {
		policy.RedrivePermission = permission
	}

	return policy
}

func findJSONValue(jsonStr, key string) string {
	// Simple JSON value extraction (not a full parser)
	keyPattern := "\"" + key + "\""
	keyIndex := -1
	for i := 0; i < len(jsonStr)-len(keyPattern); i++ {
		if jsonStr[i:i+len(keyPattern)] == keyPattern {
			keyIndex = i + len(keyPattern)
			break
		}
	}

	if keyIndex == -1 {
		return ""
	}

	// Find the colon
	colonIndex := -1
	for i := keyIndex; i < len(jsonStr); i++ {
		if jsonStr[i] == ':' {
			colonIndex = i
			break
		}
	}

	if colonIndex == -1 {
		return ""
	}

	// Find the value start
	valueStart := -1
	isString := false
	for i := colonIndex + 1; i < len(jsonStr); i++ {
		if jsonStr[i] == '"' {
			valueStart = i + 1
			isString = true
			break
		} else if jsonStr[i] >= '0' && jsonStr[i] <= '9' {
			valueStart = i
			break
		}
	}

	if valueStart == -1 {
		return ""
	}

	// Find the value end
	valueEnd := valueStart
	if isString {
		for i := valueStart; i < len(jsonStr); i++ {
			if jsonStr[i] == '"' && (i == 0 || jsonStr[i-1] != '\\') {
				valueEnd = i
				break
			}
		}
	} else {
		for i := valueStart; i < len(jsonStr); i++ {
			if jsonStr[i] == ',' || jsonStr[i] == '}' {
				valueEnd = i
				break
			}
		}
	}

	return jsonStr[valueStart:valueEnd]
}

func extractQueueNameFromArn(arn string) string {
	// ARN format: arn:aws:sqs:region:account-id:queue-name
	parts := make([]string, 0)
	current := ""
	for _, ch := range arn {
		if ch == ':' {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(ch)
		}
	}
	parts = append(parts, current)

	if len(parts) >= 6 {
		return parts[5]
	}
	return ""
}
