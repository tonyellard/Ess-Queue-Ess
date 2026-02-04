// SPDX-License-Identifier: Apache-2.0

package main

import (
	"embed"
	"encoding/json"
	"encoding/xml"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

//go:embed admin.html
var adminHTML embed.FS

var queueManager = NewQueueManager()

// SQS API Handler
func sqsHandler(w http.ResponseWriter, r *http.Request) {
	// Parse form data
	if err := r.ParseForm(); err != nil {
		sendError(w, "InvalidParameterValue", "Failed to parse request", http.StatusBadRequest)
		return
	}

	action := r.FormValue("Action")
	log.Printf("SQS Action: %s", action)

	switch action {
	case "CreateQueue":
		handleCreateQueue(w, r)
	case "DeleteQueue":
		handleDeleteQueue(w, r)
	case "ListQueues":
		handleListQueues(w, r)
	case "SendMessage":
		handleSendMessage(w, r)
	case "ReceiveMessage":
		handleReceiveMessage(w, r)
	case "DeleteMessage":
		handleDeleteMessage(w, r)
	case "GetQueueAttributes":
		handleGetQueueAttributes(w, r)
	case "PurgeQueue":
		handlePurgeQueue(w, r)
	default:
		sendError(w, "InvalidAction", "Unknown action: "+action, http.StatusBadRequest)
	}
}

func handleCreateQueue(w http.ResponseWriter, r *http.Request) {
	queueName := r.FormValue("QueueName")
	if queueName == "" {
		sendError(w, "MissingParameter", "QueueName is required", http.StatusBadRequest)
		return
	}

	attributes := parseAttributes(r.Form, "Attribute")
	queue, err := queueManager.CreateQueue(queueName, attributes)
	if err != nil {
		sendError(w, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	type CreateQueueResponse struct {
		XMLName xml.Name `xml:"CreateQueueResponse"`
		Result  struct {
			QueueUrl string `xml:"QueueUrl"`
		} `xml:"CreateQueueResult"`
	}

	resp := CreateQueueResponse{}
	resp.Result.QueueUrl = "http://" + r.Host + queue.URL

	sendXMLResponse(w, resp)
}

func handleDeleteQueue(w http.ResponseWriter, r *http.Request) {
	queueURL := r.FormValue("QueueUrl")
	queueName := extractQueueName(queueURL)

	if queueManager.DeleteQueue(queueName) {
		type DeleteQueueResponse struct {
			XMLName xml.Name `xml:"DeleteQueueResponse"`
		}
		sendXMLResponse(w, DeleteQueueResponse{})
	} else {
		sendError(w, "NonExistentQueue", "Queue does not exist", http.StatusBadRequest)
	}
}

func handleListQueues(w http.ResponseWriter, r *http.Request) {
	prefix := r.FormValue("QueueNamePrefix")
	urls := queueManager.ListQueues(prefix)

	type ListQueuesResponse struct {
		XMLName xml.Name `xml:"ListQueuesResponse"`
		Result  struct {
			QueueUrls []string `xml:"QueueUrl"`
		} `xml:"ListQueuesResult"`
	}

	resp := ListQueuesResponse{}
	for _, url := range urls {
		resp.Result.QueueUrls = append(resp.Result.QueueUrls, "http://"+r.Host+url)
	}

	sendXMLResponse(w, resp)
}

func handleSendMessage(w http.ResponseWriter, r *http.Request) {
	queueURL := r.FormValue("QueueUrl")
	queueName := extractQueueName(queueURL)
	body := r.FormValue("MessageBody")
	delaySeconds := parseIntDefault(r.FormValue("DelaySeconds"), 0)

	queue, exists := queueManager.GetQueue(queueName)
	if !exists {
		sendError(w, "NonExistentQueue", "Queue does not exist", http.StatusBadRequest)
		return
	}

	attributes := parseMessageAttributes(r.Form)
	msg := queue.SendMessage(body, attributes, delaySeconds)

	type SendMessageResponse struct {
		XMLName xml.Name `xml:"SendMessageResponse"`
		Result  struct {
			MD5OfMessageBody string `xml:"MD5OfMessageBody"`
			MessageId        string `xml:"MessageId"`
		} `xml:"SendMessageResult"`
	}

	resp := SendMessageResponse{}
	resp.Result.MD5OfMessageBody = msg.MD5OfBody
	resp.Result.MessageId = msg.MessageID

	sendXMLResponse(w, resp)
}

func handleReceiveMessage(w http.ResponseWriter, r *http.Request) {
	queueURL := r.FormValue("QueueUrl")
	queueName := extractQueueName(queueURL)
	maxMessages := parseIntDefault(r.FormValue("MaxNumberOfMessages"), 1)
	visibilityTimeout := parseIntDefault(r.FormValue("VisibilityTimeout"), 30)
	waitTimeSeconds := parseIntDefault(r.FormValue("WaitTimeSeconds"), 0)

	queue, exists := queueManager.GetQueue(queueName)
	if !exists {
		sendError(w, "NonExistentQueue", "Queue does not exist", http.StatusBadRequest)
		return
	}

	messages := queue.ReceiveMessages(maxMessages, visibilityTimeout, waitTimeSeconds)

	type MessageElement struct {
		MessageId     string `xml:"MessageId"`
		ReceiptHandle string `xml:"ReceiptHandle"`
		MD5OfBody     string `xml:"MD5OfBody"`
		Body          string `xml:"Body"`
	}

	type ReceiveMessageResponse struct {
		XMLName  xml.Name         `xml:"ReceiveMessageResponse"`
		Messages []MessageElement `xml:"ReceiveMessageResult>Message"`
	}

	resp := ReceiveMessageResponse{}
	for _, msg := range messages {
		resp.Messages = append(resp.Messages, MessageElement{
			MessageId:     msg.MessageID,
			ReceiptHandle: msg.ReceiptHandle,
			MD5OfBody:     msg.MD5OfBody,
			Body:          msg.Body,
		})
	}

	sendXMLResponse(w, resp)
}

func handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	queueURL := r.FormValue("QueueUrl")
	queueName := extractQueueName(queueURL)
	receiptHandle := r.FormValue("ReceiptHandle")

	queue, exists := queueManager.GetQueue(queueName)
	if !exists {
		sendError(w, "NonExistentQueue", "Queue does not exist", http.StatusBadRequest)
		return
	}

	if queue.DeleteMessage(receiptHandle) {
		type DeleteMessageResponse struct {
			XMLName xml.Name `xml:"DeleteMessageResponse"`
		}
		sendXMLResponse(w, DeleteMessageResponse{})
	} else {
		sendError(w, "ReceiptHandleIsInvalid", "Invalid receipt handle", http.StatusBadRequest)
	}
}

func handleGetQueueAttributes(w http.ResponseWriter, r *http.Request) {
	queueURL := r.FormValue("QueueUrl")
	queueName := extractQueueName(queueURL)

	queue, exists := queueManager.GetQueue(queueName)
	if !exists {
		sendError(w, "NonExistentQueue", "Queue does not exist", http.StatusBadRequest)
		return
	}

	attrs := queue.GetAttributes()

	type Attribute struct {
		Name  string `xml:"Name"`
		Value string `xml:"Value"`
	}

	type GetQueueAttributesResponse struct {
		XMLName xml.Name `xml:"GetQueueAttributesResponse"`
		Result  struct {
			Attributes []Attribute `xml:"Attribute"`
		} `xml:"GetQueueAttributesResult"`
	}

	resp := GetQueueAttributesResponse{}
	for name, value := range attrs {
		resp.Result.Attributes = append(resp.Result.Attributes, Attribute{
			Name:  name,
			Value: value,
		})
	}

	sendXMLResponse(w, resp)
}

func handlePurgeQueue(w http.ResponseWriter, r *http.Request) {
	queueURL := r.FormValue("QueueUrl")
	queueName := extractQueueName(queueURL)

	queue, exists := queueManager.GetQueue(queueName)
	if !exists {
		sendError(w, "NonExistentQueue", "Queue does not exist", http.StatusBadRequest)
		return
	}

	queue.PurgeQueue()

	type PurgeQueueResponse struct {
		XMLName xml.Name `xml:"PurgeQueueResponse"`
	}
	sendXMLResponse(w, PurgeQueueResponse{})
}

// Helper functions

func extractQueueName(queueURL string) string {
	parsedURL, err := url.Parse(queueURL)
	if err != nil {
		return strings.TrimPrefix(queueURL, "/")
	}
	return strings.TrimPrefix(parsedURL.Path, "/")
}

func parseAttributes(form url.Values, prefix string) map[string]string {
	attrs := make(map[string]string)
	i := 1
	for {
		nameKey := prefix + "." + strconv.Itoa(i) + ".Name"
		valueKey := prefix + "." + strconv.Itoa(i) + ".Value"

		name := form.Get(nameKey)
		value := form.Get(valueKey)

		if name == "" {
			break
		}
		attrs[name] = value
		i++
	}
	return attrs
}

func parseMessageAttributes(form url.Values) map[string]interface{} {
	// Simplified - should properly parse MessageAttribute.N.Name/Value/DataType
	return make(map[string]interface{})
}

func parseIntDefault(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	val, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return val
}

func sendXMLResponse(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(http.StatusOK)

	encoder := xml.NewEncoder(w)
	encoder.Indent("", "  ")
	if err := encoder.Encode(v); err != nil {
		log.Printf("Error encoding XML: %v", err)
	}
}

func sendError(w http.ResponseWriter, code string, message string, status int) {
	type ErrorResponse struct {
		XMLName xml.Name `xml:"ErrorResponse"`
		Error   struct {
			Type    string `xml:"Type"`
			Code    string `xml:"Code"`
			Message string `xml:"Message"`
		} `xml:"Error"`
	}

	resp := ErrorResponse{}
	resp.Error.Type = "Sender"
	resp.Error.Code = code
	resp.Error.Message = message

	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(status)

	encoder := xml.NewEncoder(w)
	encoder.Indent("", "  ")
	encoder.Encode(resp)
}

// Health check handler
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

// Root handler
func rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		sqsHandler(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	io.WriteString(w, "Ess-Queue-Ess - AWS SQS Emulator\n")
}

// Admin UI handler
func adminUIHandler(w http.ResponseWriter, r *http.Request) {
	data, err := adminHTML.ReadFile("admin.html")
	if err != nil {
		http.Error(w, "Admin UI not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(data)
}

// Admin API: Queue details
type QueueDetails struct {
	Name            string           `json:"name"`
	URL             string           `json:"url"`
	MessageCount    int              `json:"message_count"`
	VisibleCount    int              `json:"visible_count"`
	NotVisibleCount int              `json:"not_visible_count"`
	DelayedCount    int              `json:"delayed_count"`
	Messages        []MessageDetails `json:"messages"`
}

type MessageDetails struct {
	MessageID     string    `json:"message_id"`
	Body          string    `json:"body"`
	MD5OfBody     string    `json:"md5_of_body"`
	SentTimestamp time.Time `json:"sent_timestamp"`
	ReceiveCount  int       `json:"receive_count"`
	ReceiptHandle string    `json:"receipt_handle,omitempty"`
}

func adminAPIHandler(w http.ResponseWriter, r *http.Request) {
	queues := queueManager.GetAllQueues()

	queueDetails := make([]QueueDetails, 0, len(queues))
	for _, queue := range queues {
		queue.mu.RLock()

		now := time.Now()
		visibleCount := 0
		notVisibleCount := 0
		delayedCount := 0

		messages := make([]MessageDetails, 0, len(queue.Messages))
		for _, msg := range queue.Messages {
			if now.Before(msg.DelayUntil) {
				delayedCount++
			} else if now.Before(msg.VisibilityTimeout) {
				notVisibleCount++
			} else {
				visibleCount++
			}

			messages = append(messages, MessageDetails{
				MessageID:     msg.MessageID,
				Body:          msg.Body,
				MD5OfBody:     msg.MD5OfBody,
				SentTimestamp: msg.SentTimestamp,
				ReceiveCount:  msg.ReceiveCount,
				ReceiptHandle: msg.ReceiptHandle,
			})
		}

		queueDetails = append(queueDetails, QueueDetails{
			Name:            queue.Name,
			URL:             queue.URL,
			MessageCount:    len(queue.Messages),
			VisibleCount:    visibleCount,
			NotVisibleCount: notVisibleCount,
			DelayedCount:    delayedCount,
			Messages:        messages,
		})

		queue.mu.RUnlock()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"queues": queueDetails,
	})
}
