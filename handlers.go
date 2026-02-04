// SPDX-License-Identifier: Apache-2.0

package main

import (
	"embed"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

//go:embed admin.html
var adminHTML embed.FS

var queueManager = NewQueueManager()

// SQS API Handler
func sqsHandler(w http.ResponseWriter, r *http.Request) {
	var action string

	// AWS CLI/SDK can send requests in multiple formats:
	// 1. Query protocol (form-encoded) - older style
	// 2. JSON protocol with X-Amz-Target header - newer AWS CLI default

	// Check for X-Amz-Target header (JSON protocol)
	target := r.Header.Get("X-Amz-Target")
	if target != "" {
		// Extract action from target like "AmazonSQS.CreateQueue"
		parts := strings.Split(target, ".")
		if len(parts) == 2 {
			action = parts[1]
		}
	} else {
		// Fall back to Query protocol (form-encoded)
		if err := r.ParseForm(); err != nil {
			sendError(w, "InvalidParameterValue", "Failed to parse request", http.StatusBadRequest)
			return
		}
		action = r.FormValue("Action")
	}

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
	case "StartMessageMoveTask":
		handleStartMessageMoveTask(w, r)
	case "ListMessageMoveTasks":
		handleListMessageMoveTasks(w, r)
	case "CancelMessageMoveTask":
		handleCancelMessageMoveTask(w, r)
	default:
		sendError(w, "InvalidAction", "Unknown action: "+action, http.StatusBadRequest)
	}
}

// getRequestParam extracts a parameter from either JSON body or form data
func getRequestParam(r *http.Request, paramName string) string {
	// Check if this is a JSON request (X-Amz-Target header present)
	if r.Header.Get("X-Amz-Target") != "" {
		// Parse JSON body
		var jsonBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&jsonBody); err == nil {
			if val, ok := jsonBody[paramName]; ok {
				if strVal, ok := val.(string); ok {
					return strVal
				}
			}
		}
		// Reset body for potential re-reads
		return ""
	}

	// Fall back to form data
	if r.Form == nil {
		r.ParseForm()
	}
	return r.FormValue(paramName)
}

// parseRequestJSON parses the JSON body into a map
func parseRequestJSON(r *http.Request) (map[string]interface{}, error) {
	var jsonBody map[string]interface{}

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	// Reset body for potential re-reads
	r.Body = io.NopCloser(strings.NewReader(string(body)))

	// Parse JSON
	if err := json.Unmarshal(body, &jsonBody); err != nil {
		return nil, err
	}

	return jsonBody, nil
}

func handleCreateQueue(w http.ResponseWriter, r *http.Request) {
	var queueName string
	var attributes map[string]string

	// Check if this is a JSON request
	if r.Header.Get("X-Amz-Target") != "" {
		jsonBody, err := parseRequestJSON(r)
		if err != nil {
			sendError(w, "InvalidParameterValue", "Failed to parse JSON request", http.StatusBadRequest)
			return
		}

		if name, ok := jsonBody["QueueName"].(string); ok {
			queueName = name
		}

		// Parse attributes from JSON
		attributes = make(map[string]string)
		if attrs, ok := jsonBody["Attributes"].(map[string]interface{}); ok {
			for k, v := range attrs {
				if strVal, ok := v.(string); ok {
					attributes[k] = strVal
				}
			}
		}
	} else {
		// Form-encoded request
		if err := r.ParseForm(); err != nil {
			sendError(w, "InvalidParameterValue", "Failed to parse request", http.StatusBadRequest)
			return
		}
		queueName = r.FormValue("QueueName")
		attributes = parseAttributes(r.Form, "Attribute")
	}

	if queueName == "" {
		sendError(w, "MissingParameter", "QueueName is required", http.StatusBadRequest)
		return
	}

	queue, err := queueManager.CreateQueue(queueName, attributes)
	if err != nil {
		sendError(w, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	type CreateQueueResponse struct {
		XMLName xml.Name `xml:"CreateQueueResponse" json:"-"`
		Result  struct {
			QueueUrl string `xml:"QueueUrl" json:"QueueUrl"`
		} `xml:"CreateQueueResult" json:"-"`
	}

	type CreateQueueJSONResponse struct {
		QueueUrl string `json:"QueueUrl"`
	}

	resp := CreateQueueResponse{}
	resp.Result.QueueUrl = "http://" + r.Host + queue.URL

	jsonResp := CreateQueueJSONResponse{
		QueueUrl: "http://" + r.Host + queue.URL,
	}

	sendResponse(w, r, resp, jsonResp)
}

func handleDeleteQueue(w http.ResponseWriter, r *http.Request) {
	var queueURL string

	// Check if this is a JSON request
	if r.Header.Get("X-Amz-Target") != "" {
		jsonBody, err := parseRequestJSON(r)
		if err != nil {
			sendError(w, "InvalidParameterValue", "Failed to parse JSON request", http.StatusBadRequest)
			return
		}

		if url, ok := jsonBody["QueueUrl"].(string); ok {
			queueURL = url
		}
	} else {
		// Form-encoded request
		if err := r.ParseForm(); err != nil {
			sendError(w, "InvalidParameterValue", "Failed to parse request", http.StatusBadRequest)
			return
		}
		queueURL = r.FormValue("QueueUrl")
	}

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
	var prefix string

	// Check if this is a JSON request
	if r.Header.Get("X-Amz-Target") != "" {
		jsonBody, err := parseRequestJSON(r)
		if err != nil {
			sendError(w, "InvalidParameterValue", "Failed to parse JSON request", http.StatusBadRequest)
			return
		}

		if p, ok := jsonBody["QueueNamePrefix"].(string); ok {
			prefix = p
		}
	} else {
		// Form-encoded request
		if err := r.ParseForm(); err != nil {
			sendError(w, "InvalidParameterValue", "Failed to parse request", http.StatusBadRequest)
			return
		}
		prefix = r.FormValue("QueueNamePrefix")
	}

	urls := queueManager.ListQueues(prefix)

	type ListQueuesResponse struct {
		XMLName xml.Name `xml:"ListQueuesResponse" json:"-"`
		Result  struct {
			QueueUrls []string `xml:"QueueUrl" json:"QueueUrls"`
		} `xml:"ListQueuesResult" json:"-"`
	}

	type ListQueuesJSONResponse struct {
		QueueUrls []string `json:"QueueUrls"`
	}

	resp := ListQueuesResponse{}
	fullUrls := []string{}
	for _, url := range urls {
		fullUrl := "http://" + r.Host + url
		resp.Result.QueueUrls = append(resp.Result.QueueUrls, fullUrl)
		fullUrls = append(fullUrls, fullUrl)
	}

	jsonResp := ListQueuesJSONResponse{
		QueueUrls: fullUrls,
	}

	sendResponse(w, r, resp, jsonResp)
}

func handleSendMessage(w http.ResponseWriter, r *http.Request) {
	var queueURL, body string
	var delaySeconds int
	var attributes map[string]interface{}
	var deduplicationId, groupId string

	// Check if this is a JSON request
	if r.Header.Get("X-Amz-Target") != "" {
		jsonBody, err := parseRequestJSON(r)
		if err != nil {
			sendError(w, "InvalidParameterValue", "Failed to parse JSON request", http.StatusBadRequest)
			return
		}

		if url, ok := jsonBody["QueueUrl"].(string); ok {
			queueURL = url
		}
		if msgBody, ok := jsonBody["MessageBody"].(string); ok {
			body = msgBody
		}
		if delay, ok := jsonBody["DelaySeconds"].(float64); ok {
			delaySeconds = int(delay)
		}
		if attrs, ok := jsonBody["MessageAttributes"].(map[string]interface{}); ok {
			attributes = attrs
		} else {
			attributes = make(map[string]interface{})
		}
		// FIFO-specific parameters
		if dedupId, ok := jsonBody["MessageDeduplicationId"].(string); ok {
			deduplicationId = dedupId
		}
		if msgGroupId, ok := jsonBody["MessageGroupId"].(string); ok {
			groupId = msgGroupId
		}
	} else {
		// Form-encoded request
		if err := r.ParseForm(); err != nil {
			sendError(w, "InvalidParameterValue", "Failed to parse request", http.StatusBadRequest)
			return
		}
		queueURL = r.FormValue("QueueUrl")
		body = r.FormValue("MessageBody")
		delaySeconds = parseIntDefault(r.FormValue("DelaySeconds"), 0)
		attributes = parseMessageAttributes(r.Form)
		deduplicationId = r.FormValue("MessageDeduplicationId")
		groupId = r.FormValue("MessageGroupId")
	}

	queueName := extractQueueName(queueURL)

	queue, exists := queueManager.GetQueue(queueName)
	if !exists {
		sendError(w, "NonExistentQueue", "Queue does not exist", http.StatusBadRequest)
		return
	}

	msg := queue.SendMessage(body, attributes, delaySeconds, deduplicationId, groupId)

	type SendMessageResponse struct {
		XMLName xml.Name `xml:"SendMessageResponse" json:"-"`
		Result  struct {
			MD5OfMessageBody string `xml:"MD5OfMessageBody" json:"MD5OfMessageBody"`
			MessageId        string `xml:"MessageId" json:"MessageId"`
			SequenceNumber   string `xml:"SequenceNumber,omitempty" json:"SequenceNumber,omitempty"`
		} `xml:"SendMessageResult" json:"-"`
	}

	type SendMessageJSONResponse struct {
		MD5OfMessageBody string `json:"MD5OfMessageBody"`
		MessageId        string `json:"MessageId"`
		SequenceNumber   string `json:"SequenceNumber,omitempty"`
	}

	resp := SendMessageResponse{}
	resp.Result.MD5OfMessageBody = msg.MD5OfBody
	resp.Result.MessageId = msg.MessageID
	if msg.SequenceNumber != "" {
		resp.Result.SequenceNumber = msg.SequenceNumber
	}

	jsonResp := SendMessageJSONResponse{
		MD5OfMessageBody: msg.MD5OfBody,
		MessageId:        msg.MessageID,
		SequenceNumber:   msg.SequenceNumber,
	}

	sendResponse(w, r, resp, jsonResp)
}

func handleReceiveMessage(w http.ResponseWriter, r *http.Request) {
	var queueURL string
	var maxMessages, visibilityTimeout int

	// Check if this is a JSON request
	if r.Header.Get("X-Amz-Target") != "" {
		jsonBody, err := parseRequestJSON(r)
		if err != nil {
			sendError(w, "InvalidParameterValue", "Failed to parse JSON request", http.StatusBadRequest)
			return
		}

		if url, ok := jsonBody["QueueUrl"].(string); ok {
			queueURL = url
		}
		if max, ok := jsonBody["MaxNumberOfMessages"].(float64); ok {
			maxMessages = int(max)
		} else {
			maxMessages = 1
		}
		if vis, ok := jsonBody["VisibilityTimeout"].(float64); ok {
			visibilityTimeout = int(vis)
		} else {
			visibilityTimeout = 30
		}
	} else {
		// Form-encoded request
		if err := r.ParseForm(); err != nil {
			sendError(w, "InvalidParameterValue", "Failed to parse request", http.StatusBadRequest)
			return
		}
		queueURL = r.FormValue("QueueUrl")
		maxMessages = parseIntDefault(r.FormValue("MaxNumberOfMessages"), 1)
		visibilityTimeout = parseIntDefault(r.FormValue("VisibilityTimeout"), 30)
	}

	queueName := extractQueueName(queueURL)
	waitTimeSeconds := parseIntDefault(r.FormValue("WaitTimeSeconds"), 0)

	queue, exists := queueManager.GetQueue(queueName)
	if !exists {
		sendError(w, "NonExistentQueue", "Queue does not exist", http.StatusBadRequest)
		return
	}

	messages := queue.ReceiveMessages(maxMessages, visibilityTimeout, waitTimeSeconds)

	type MessageElement struct {
		MessageId     string `xml:"MessageId" json:"MessageId"`
		ReceiptHandle string `xml:"ReceiptHandle" json:"ReceiptHandle"`
		MD5OfBody     string `xml:"MD5OfBody" json:"MD5OfBody"`
		Body          string `xml:"Body" json:"Body"`
	}

	type ReceiveMessageResponse struct {
		XMLName  xml.Name         `xml:"ReceiveMessageResponse" json:"-"`
		Messages []MessageElement `xml:"ReceiveMessageResult>Message" json:"Messages"`
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

	// Send JSON or XML based on request type
	sendResponse(w, r, resp, resp)
}

func handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	var queueURL, receiptHandle string
	isJSON := r.Header.Get("X-Amz-Target") != ""

	// Check if this is a JSON request
	if isJSON {
		jsonBody, err := parseRequestJSON(r)
		if err != nil {
			sendError(w, "InvalidParameterValue", "Failed to parse JSON request", http.StatusBadRequest)
			return
		}

		if url, ok := jsonBody["QueueUrl"].(string); ok {
			queueURL = url
		}
		if receipt, ok := jsonBody["ReceiptHandle"].(string); ok {
			receiptHandle = receipt
		}
	} else {
		// Form-encoded request
		if err := r.ParseForm(); err != nil {
			sendError(w, "InvalidParameterValue", "Failed to parse request", http.StatusBadRequest)
			return
		}
		queueURL = r.FormValue("QueueUrl")
		receiptHandle = r.FormValue("ReceiptHandle")
	}

	queueName := extractQueueName(queueURL)

	queue, exists := queueManager.GetQueue(queueName)
	if !exists {
		sendError(w, "NonExistentQueue", "Queue does not exist", http.StatusBadRequest)
		return
	}

	if queue.DeleteMessage(receiptHandle) {
		if isJSON {
			sendJSONResponse(w, struct{}{})
		} else {
			type DeleteMessageResponse struct {
				XMLName xml.Name `xml:"DeleteMessageResponse"`
			}
			sendXMLResponse(w, DeleteMessageResponse{})
		}
	} else {
		sendError(w, "ReceiptHandleIsInvalid", "Invalid receipt handle", http.StatusBadRequest)
	}
}

func handleGetQueueAttributes(w http.ResponseWriter, r *http.Request) {
	var queueURL string
	isJSON := r.Header.Get("X-Amz-Target") != ""

	// Check if this is a JSON request
	if isJSON {
		jsonBody, err := parseRequestJSON(r)
		if err != nil {
			sendError(w, "InvalidParameterValue", "Failed to parse JSON request", http.StatusBadRequest)
			return
		}

		if url, ok := jsonBody["QueueUrl"].(string); ok {
			queueURL = url
		}
	} else {
		// Form-encoded request
		if err := r.ParseForm(); err != nil {
			sendError(w, "InvalidParameterValue", "Failed to parse request", http.StatusBadRequest)
			return
		}
		queueURL = r.FormValue("QueueUrl")
	}

	queueName := extractQueueName(queueURL)

	queue, exists := queueManager.GetQueue(queueName)
	if !exists {
		sendError(w, "NonExistentQueue", "Queue does not exist", http.StatusBadRequest)
		return
	}

	attrs := queue.GetAttributes()

	if isJSON {
		// JSON response for AWS SDK
		type GetQueueAttributesJSONResponse struct {
			Attributes map[string]string `json:"Attributes"`
		}
		resp := GetQueueAttributesJSONResponse{
			Attributes: attrs,
		}
		sendJSONResponse(w, resp)
	} else {
		// XML response for Query protocol
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
}

func handlePurgeQueue(w http.ResponseWriter, r *http.Request) {
	var queueURL string

	// Check if this is a JSON request
	if r.Header.Get("X-Amz-Target") != "" {
		jsonBody, err := parseRequestJSON(r)
		if err != nil {
			sendError(w, "InvalidParameterValue", "Failed to parse JSON request", http.StatusBadRequest)
			return
		}

		if url, ok := jsonBody["QueueUrl"].(string); ok {
			queueURL = url
		}
	} else {
		// Form-encoded request
		if err := r.ParseForm(); err != nil {
			sendError(w, "InvalidParameterValue", "Failed to parse request", http.StatusBadRequest)
			return
		}
		queueURL = r.FormValue("QueueUrl")
	}

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

func sendJSONResponse(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.0")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("Error encoding JSON: %v", err)
	}
}

func sendResponse(w http.ResponseWriter, r *http.Request, xmlData interface{}, jsonData interface{}) {
	// If X-Amz-Target header is present, send JSON response
	if r.Header.Get("X-Amz-Target") != "" {
		sendJSONResponse(w, jsonData)
	} else {
		sendXMLResponse(w, xmlData)
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
	Name                      string              `json:"name"`
	URL                       string              `json:"url"`
	MessageCount              int                 `json:"message_count"`
	VisibleCount              int                 `json:"visible_count"`
	NotVisibleCount           int                 `json:"not_visible_count"`
	DelayedCount              int                 `json:"delayed_count"`
	Messages                  []MessageDetails    `json:"messages"`
	FifoQueue                 bool                `json:"fifo_queue"`
	ContentBasedDeduplication bool                `json:"content_based_deduplication,omitempty"`
	RedrivePolicy             *RedrivePolicy      `json:"redrive_policy,omitempty"`
	RedriveAllowPolicy        *RedriveAllowPolicy `json:"redrive_allow_policy,omitempty"`
}

type MessageDetails struct {
	MessageID              string    `json:"message_id"`
	Body                   string    `json:"body"`
	MD5OfBody              string    `json:"md5_of_body"`
	SentTimestamp          time.Time `json:"sent_timestamp"`
	ReceiveCount           int       `json:"receive_count"`
	ReceiptHandle          string    `json:"receipt_handle,omitempty"`
	SequenceNumber         string    `json:"sequence_number,omitempty"`
	MessageGroupId         string    `json:"message_group_id,omitempty"`
	MessageDeduplicationId string    `json:"message_deduplication_id,omitempty"`
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
				MessageID:              msg.MessageID,
				Body:                   msg.Body,
				MD5OfBody:              msg.MD5OfBody,
				SentTimestamp:          msg.SentTimestamp,
				ReceiveCount:           msg.ReceiveCount,
				ReceiptHandle:          msg.ReceiptHandle,
				SequenceNumber:         msg.SequenceNumber,
				MessageGroupId:         msg.MessageGroupId,
				MessageDeduplicationId: msg.MessageDeduplicationId,
			})
		}

		queueDetails = append(queueDetails, QueueDetails{
			Name:                      queue.Name,
			URL:                       queue.URL,
			MessageCount:              len(queue.Messages),
			VisibleCount:              visibleCount,
			NotVisibleCount:           notVisibleCount,
			DelayedCount:              delayedCount,
			Messages:                  messages,
			FifoQueue:                 queue.FifoQueue,
			ContentBasedDeduplication: queue.ContentBasedDeduplication,
			RedrivePolicy:             queue.RedrivePolicy,
			RedriveAllowPolicy:        queue.RedriveAllowPolicy,
		})

		queue.mu.RUnlock()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"queues": queueDetails,
	})
}

// adminCreateQueueHandler creates a new queue via the admin API
func adminCreateQueueHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name                   string            `json:"name"`
		VisibilityTimeout      int               `json:"visibility_timeout"`
		MessageRetentionPeriod int               `json:"message_retention_period"`
		MaxMessageSize         int               `json:"max_message_size"`
		Attributes             map[string]string `json:"attributes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Queue name is required", http.StatusBadRequest)
		return
	}

	// Set defaults if not provided
	if req.VisibilityTimeout == 0 {
		req.VisibilityTimeout = 30
	}
	if req.MessageRetentionPeriod == 0 {
		req.MessageRetentionPeriod = 345600 // 4 days in seconds
	}
	if req.MaxMessageSize == 0 {
		req.MaxMessageSize = 262144 // 256 KB
	}

	// Build attributes map
	attributes := make(map[string]string)
	attributes["VisibilityTimeout"] = strconv.Itoa(req.VisibilityTimeout)
	attributes["MessageRetentionPeriod"] = strconv.Itoa(req.MessageRetentionPeriod)
	attributes["MaximumMessageSize"] = strconv.Itoa(req.MaxMessageSize)

	// Merge in any additional attributes from the request (FIFO, RedrivePolicy, etc.)
	for k, v := range req.Attributes {
		attributes[k] = v
	}

	queue, err := queueManager.CreateQueue(req.Name, attributes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update queue settings
	queue.mu.Lock()
	queue.VisibilityTimeout = req.VisibilityTimeout
	queue.MessageRetentionPeriod = req.MessageRetentionPeriod
	queue.MaximumMessageSize = req.MaxMessageSize
	queue.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"queue": map[string]interface{}{
			"name":                     queue.Name,
			"url":                      queue.URL,
			"visibility_timeout":       queue.VisibilityTimeout,
			"message_retention_period": queue.MessageRetentionPeriod,
			"maximum_message_size":     queue.MaximumMessageSize,
		},
	})
}

// adminDeleteQueueHandler deletes a queue via the admin API
func adminDeleteQueueHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	queueName := r.URL.Query().Get("name")
	if queueName == "" {
		http.Error(w, "Queue name is required", http.StatusBadRequest)
		return
	}

	queueManager.DeleteQueue(queueName)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Queue '%s' deleted successfully", queueName),
	})
}

// adminSendMessageHandler sends a test message to a queue via the admin API
func adminSendMessageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		QueueName              string            `json:"queue_name"`
		MessageBody            string            `json:"message_body"`
		DelaySeconds           int               `json:"delay_seconds"`
		Attributes             map[string]string `json:"attributes"`
		MessageGroupId         string            `json:"message_group_id"`
		MessageDeduplicationId string            `json:"message_deduplication_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.QueueName == "" || req.MessageBody == "" {
		http.Error(w, "Queue name and message body are required", http.StatusBadRequest)
		return
	}

	queue, exists := queueManager.GetQueue(req.QueueName)
	if !exists {
		http.Error(w, "Queue not found", http.StatusNotFound)
		return
	}

	// Convert string map to interface map for attributes
	attrs := make(map[string]interface{})
	for k, v := range req.Attributes {
		attrs[k] = v
	}

	message := queue.SendMessage(req.MessageBody, attrs, req.DelaySeconds, req.MessageDeduplicationId, req.MessageGroupId)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":         true,
		"message_id":      message.MessageID,
		"sequence_number": message.SequenceNumber,
		"queue_name":      req.QueueName,
	})
}

// adminExportConfigHandler exports the current queue configuration as YAML
func adminExportConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	queues := queueManager.GetAllQueues()

	var configYAML strings.Builder
	configYAML.WriteString("# Ess-Queue-Ess Configuration\n")
	configYAML.WriteString("# Generated on: " + time.Now().Format(time.RFC3339) + "\n\n")
	configYAML.WriteString("server:\n")
	configYAML.WriteString("  port: 9324\n")
	configYAML.WriteString("  host: 0.0.0.0\n\n")
	configYAML.WriteString("queues:\n")

	for _, queue := range queues {
		queue.mu.RLock()
		configYAML.WriteString(fmt.Sprintf("  - name: %s\n", queue.Name))
		configYAML.WriteString(fmt.Sprintf("    visibility_timeout: %d\n", queue.VisibilityTimeout))
		configYAML.WriteString(fmt.Sprintf("    message_retention_period: %d\n", queue.MessageRetentionPeriod))
		configYAML.WriteString(fmt.Sprintf("    maximum_message_size: %d\n", queue.MaximumMessageSize))
		queue.mu.RUnlock()
	}

	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("Content-Disposition", "attachment; filename=config.yaml")
	w.Write([]byte(configYAML.String()))
}

// Redrive handlers for DLQ support
func handleStartMessageMoveTask(w http.ResponseWriter, r *http.Request) {
	var sourceArn string
	var destinationArn string
	var maxMessages int

	isJSON := r.Header.Get("X-Amz-Target") != ""

	if isJSON {
		jsonBody, err := parseRequestJSON(r)
		if err != nil {
			sendError(w, "InvalidParameterValue", "Failed to parse JSON request", http.StatusBadRequest)
			return
		}

		if arn, ok := jsonBody["SourceArn"].(string); ok {
			sourceArn = arn
		}
		if arn, ok := jsonBody["DestinationArn"].(string); ok {
			destinationArn = arn
		}
		if max, ok := jsonBody["MaxNumberOfMessagesPerSecond"].(float64); ok {
			maxMessages = int(max)
		}
	} else {
		if err := r.ParseForm(); err != nil {
			sendError(w, "InvalidParameterValue", "Failed to parse request", http.StatusBadRequest)
			return
		}
		sourceArn = r.FormValue("SourceArn")
		destinationArn = r.FormValue("DestinationArn")
		maxMessages = parseIntDefault(r.FormValue("MaxNumberOfMessagesPerSecond"), 0)
	}

	// Extract queue names from ARNs
	sourceName := extractQueueNameFromArn(sourceArn)

	// If destinationArn is empty, use the source queue's redrive policy
	var destName string
	if destinationArn != "" {
		destName = extractQueueNameFromArn(destinationArn)
	} else {
		// Get the source queue from DLQ and find which queue has this as their DLQ
		_, exists := queueManager.GetQueue(sourceName)
		if !exists {
			sendError(w, "NonExistentQueue", "Source queue does not exist", http.StatusBadRequest)
			return
		}

		// Find which queue has this as their DLQ
		for _, q := range queueManager.GetAllQueues() {
			if q.RedrivePolicy != nil && q.RedrivePolicy.DeadLetterTargetArn == sourceArn {
				destName = q.Name
				break
			}
		}
	}

	if maxMessages == 0 {
		maxMessages = 100 // Default to moving 100 messages
	}

	movedCount := queueManager.RedriveMessages(sourceName, "arn:aws:sqs:us-east-1:000000000000:"+destName, maxMessages)

	taskId := uuid.New().String()

	if isJSON {
		type StartMessageMoveTaskJSONResponse struct {
			TaskHandle string `json:"TaskHandle"`
		}
		resp := StartMessageMoveTaskJSONResponse{
			TaskHandle: taskId,
		}
		sendJSONResponse(w, resp)
	} else {
		type StartMessageMoveTaskResponse struct {
			XMLName xml.Name `xml:"StartMessageMoveTaskResponse"`
			Result  struct {
				TaskHandle string `xml:"TaskHandle"`
			} `xml:"StartMessageMoveTaskResult"`
		}
		resp := StartMessageMoveTaskResponse{}
		resp.Result.TaskHandle = taskId
		sendXMLResponse(w, resp)
	}

	log.Printf("Started message move task %s: moved %d messages from %s to %s", taskId, movedCount, sourceName, destName)
}

func handleListMessageMoveTasks(w http.ResponseWriter, r *http.Request) {
	isJSON := r.Header.Get("X-Amz-Target") != ""

	// For now, return empty list since we process moves immediately
	if isJSON {
		type ListMessageMoveTasksJSONResponse struct {
			Results []interface{} `json:"Results"`
		}
		resp := ListMessageMoveTasksJSONResponse{
			Results: make([]interface{}, 0),
		}
		sendJSONResponse(w, resp)
	} else {
		type ListMessageMoveTasksResponse struct {
			XMLName xml.Name `xml:"ListMessageMoveTasksResponse"`
			Result  struct {
				Results []interface{} `xml:"Results"`
			} `xml:"ListMessageMoveTasksResult"`
		}
		resp := ListMessageMoveTasksResponse{}
		resp.Result.Results = make([]interface{}, 0)
		sendXMLResponse(w, resp)
	}
}

func handleCancelMessageMoveTask(w http.ResponseWriter, r *http.Request) {
	isJSON := r.Header.Get("X-Amz-Target") != ""

	// Since we process moves immediately, there's nothing to cancel
	if isJSON {
		sendJSONResponse(w, struct{}{})
	} else {
		type CancelMessageMoveTaskResponse struct {
			XMLName xml.Name `xml:"CancelMessageMoveTaskResponse"`
		}
		sendXMLResponse(w, CancelMessageMoveTaskResponse{})
	}
}
