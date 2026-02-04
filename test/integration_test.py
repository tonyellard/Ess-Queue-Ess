#!/usr/bin/env python3
"""
Integration tests for Ess-Queue-Ess SQS emulator.
Tests core SQS operations and admin UI functionality.
"""

import json
import requests
import sys
import time
from urllib.parse import urlencode

BASE_URL = "http://localhost:9324"
ADMIN_URL = f"{BASE_URL}/admin"
API_URL = f"{BASE_URL}/admin/api/queues"

class Colors:
    GREEN = '\033[92m'
    RED = '\033[91m'
    YELLOW = '\033[93m'
    BLUE = '\033[94m'
    END = '\033[0m'

def print_test(name):
    print(f"\n{Colors.BLUE}▶{Colors.END} Testing: {name}")

def print_success(msg):
    print(f"  {Colors.GREEN}✓{Colors.END} {msg}")

def print_error(msg):
    print(f"  {Colors.RED}✗{Colors.END} {msg}")

def print_info(msg):
    print(f"  {Colors.YELLOW}ℹ{Colors.END} {msg}")

def sqs_request(action, params=None):
    """Make an SQS API request"""
    if params is None:
        params = {}
    params['Action'] = action
    
    response = requests.post(BASE_URL, data=params)
    return response

def test_health_check():
    print_test("Health Check")
    response = requests.get(f"{BASE_URL}/health")
    assert response.status_code == 200, f"Health check failed: {response.status_code}"
    data = response.json()
    assert data.get('status') == 'healthy', f"Unexpected health status: {data}"
    print_success("Health endpoint returns healthy status")

def test_admin_ui():
    print_test("Admin UI")
    response = requests.get(ADMIN_URL)
    assert response.status_code == 200, f"Admin UI failed: {response.status_code}"
    assert 'Ess-Queue-Ess Admin' in response.text, "Admin UI HTML not found"
    print_success("Admin UI loads successfully")

def test_admin_api():
    print_test("Admin API")
    response = requests.get(API_URL)
    assert response.status_code == 200, f"Admin API failed: {response.status_code}"
    data = response.json()
    assert 'queues' in data, "Admin API missing queues field"
    print_success(f"Admin API returns {len(data['queues'])} queues")
    return data['queues']

def test_create_queue():
    print_test("Create Queue")
    queue_name = "test-integration-queue"
    response = sqs_request('CreateQueue', {'QueueName': queue_name})
    assert response.status_code == 200, f"Create queue failed: {response.status_code}"
    assert queue_name in response.text, f"Queue name not in response: {response.text}"
    print_success(f"Queue '{queue_name}' created successfully")
    return queue_name

def test_list_queues(expected_count=None):
    print_test("List Queues")
    response = sqs_request('ListQueues')
    assert response.status_code == 200, f"List queues failed: {response.status_code}"
    queue_count = response.text.count('<QueueUrl>')
    if expected_count is not None:
        assert queue_count >= expected_count, f"Expected at least {expected_count} queues, got {queue_count}"
    print_success(f"Listed {queue_count} queues")
    return queue_count

def test_send_message(queue_name):
    print_test("Send Message")
    queue_url = f"{BASE_URL}/{queue_name}"
    message_body = "Test message from integration tests"
    
    response = sqs_request('SendMessage', {
        'QueueUrl': queue_url,
        'MessageBody': message_body
    })
    
    assert response.status_code == 200, f"Send message failed: {response.status_code}"
    assert 'MessageId' in response.text, "MessageId not in response"
    print_success(f"Message sent to '{queue_name}'")

def test_send_multiple_messages(queue_name, count=5):
    print_test(f"Send {count} Messages")
    queue_url = f"{BASE_URL}/{queue_name}"
    
    for i in range(count):
        response = sqs_request('SendMessage', {
            'QueueUrl': queue_url,
            'MessageBody': f"Test message #{i+1}"
        })
        assert response.status_code == 200, f"Send message {i+1} failed"
    
    print_success(f"Sent {count} messages to '{queue_name}'")

def test_receive_message(queue_name, expected_count=1):
    print_test("Receive Messages")
    queue_url = f"{BASE_URL}/{queue_name}"
    
    response = sqs_request('ReceiveMessage', {
        'QueueUrl': queue_url,
        'MaxNumberOfMessages': '10'
    })
    
    assert response.status_code == 200, f"Receive message failed: {response.status_code}"
    message_count = response.text.count('<MessageId>')
    assert message_count >= expected_count, f"Expected at least {expected_count} messages, got {message_count}"
    print_success(f"Received {message_count} messages from '{queue_name}'")
    return response.text

def test_delete_message(queue_name):
    print_test("Delete Message")
    queue_url = f"{BASE_URL}/{queue_name}"
    
    # First receive a message to get receipt handle
    response = sqs_request('ReceiveMessage', {
        'QueueUrl': queue_url,
        'MaxNumberOfMessages': '1'
    })
    
    if '<ReceiptHandle>' not in response.text:
        print_info("No messages to delete (queue is empty)")
        return
    
    # Extract receipt handle (simple parsing)
    start = response.text.find('<ReceiptHandle>') + len('<ReceiptHandle>')
    end = response.text.find('</ReceiptHandle>')
    receipt_handle = response.text[start:end]
    
    # Delete the message
    response = sqs_request('DeleteMessage', {
        'QueueUrl': queue_url,
        'ReceiptHandle': receipt_handle
    })
    
    assert response.status_code == 200, f"Delete message failed: {response.status_code}"
    print_success(f"Message deleted from '{queue_name}'")

def test_get_queue_attributes(queue_name):
    print_test("Get Queue Attributes")
    queue_url = f"{BASE_URL}/{queue_name}"
    
    response = sqs_request('GetQueueAttributes', {
        'QueueUrl': queue_url,
        'AttributeName.1': 'All'
    })
    
    assert response.status_code == 200, f"Get attributes failed: {response.status_code}"
    assert 'ApproximateNumberOfMessages' in response.text, "Attributes not in response"
    print_success(f"Retrieved attributes for '{queue_name}'")

def test_purge_queue(queue_name):
    print_test("Purge Queue")
    queue_url = f"{BASE_URL}/{queue_name}"
    
    response = sqs_request('PurgeQueue', {
        'QueueUrl': queue_url
    })
    
    assert response.status_code == 200, f"Purge queue failed: {response.status_code}"
    print_success(f"Queue '{queue_name}' purged")
    
    # Verify queue is empty
    time.sleep(0.5)  # Brief pause
    response = sqs_request('ReceiveMessage', {
        'QueueUrl': queue_url,
        'MaxNumberOfMessages': '10'
    })
    message_count = response.text.count('<MessageId>')
    assert message_count == 0, f"Queue not empty after purge: {message_count} messages"
    print_success("Verified queue is empty after purge")

def test_delete_queue(queue_name):
    print_test("Delete Queue")
    queue_url = f"{BASE_URL}/{queue_name}"
    
    response = sqs_request('DeleteQueue', {
        'QueueUrl': queue_url
    })
    
    assert response.status_code == 200, f"Delete queue failed: {response.status_code}"
    print_success(f"Queue '{queue_name}' deleted")

def test_admin_api_with_messages():
    print_test("Admin API with Messages")
    
    # Create queue and add messages
    queue_name = "admin-test-queue"
    response = sqs_request('CreateQueue', {'QueueName': queue_name})
    queue_url = f"{BASE_URL}/{queue_name}"
    
    # Send 3 messages
    for i in range(3):
        sqs_request('SendMessage', {
            'QueueUrl': queue_url,
            'MessageBody': f"Admin test message #{i+1}"
        })
    
    time.sleep(0.5)  # Brief pause for messages to be processed
    
    # Check admin API
    response = requests.get(API_URL)
    data = response.json()
    
    # Find our queue in the response
    test_queue = None
    for q in data['queues']:
        if q['name'] == queue_name:
            test_queue = q
            break
    
    assert test_queue is not None, f"Queue '{queue_name}' not found in admin API"
    assert test_queue['visible_count'] == 3, f"Expected 3 visible messages, got {test_queue['visible_count']}"
    assert len(test_queue['messages']) == 3, f"Expected 3 messages in data, got {len(test_queue['messages'])}"
    
    print_success(f"Admin API correctly shows 3 messages in '{queue_name}'")
    
    # Cleanup
    sqs_request('DeleteQueue', {'QueueUrl': queue_url})

def test_admin_create_queue():
    print_test("Admin API - Create Queue")
    
    queue_data = {
        "name": "admin-test-queue",
        "visibility_timeout": 60,
        "message_retention_period": 86400,
        "max_message_size": 131072
    }
    
    response = requests.post(f"{BASE_URL}/admin/api/queue", json=queue_data)
    assert response.status_code == 200, f"Failed to create queue via admin API: {response.text}"
    
    data = response.json()
    assert data['success'] == True, "Expected success=true"
    assert data['queue']['name'] == "admin-test-queue", f"Queue name mismatch: {data}"
    assert data['queue']['visibility_timeout'] == 60, f"Visibility timeout mismatch: {data}"
    
    print_success("Queue created successfully via admin API")
    
def test_admin_send_message():
    print_test("Admin API - Send Message")
    
    message_data = {
        "queue_name": "admin-test-queue",
        "message_body": "Test message from admin API",
        "delay_seconds": 0,
        "attributes": {}
    }
    
    response = requests.post(f"{BASE_URL}/admin/api/message", json=message_data)
    assert response.status_code == 200, f"Failed to send message via admin API: {response.text}"
    
    data = response.json()
    assert data['success'] == True, "Expected success=true"
    assert 'message_id' in data, "Missing message_id in response"
    
    print_success(f"Message sent successfully via admin API (ID: {data['message_id']})")
    
    # Verify message was sent by checking admin API
    response = requests.get(f"{BASE_URL}/admin/api/queues")
    data = response.json()
    queues = {q['name']: q for q in data['queues']}
    
    admin_queue = queues.get('admin-test-queue')
    assert admin_queue is not None, "Queue not found in admin API"
    assert admin_queue['visible_count'] == 1, f"Expected 1 visible message, got {admin_queue['visible_count']}"
    
    print_success("Message visible in queue via admin API")

def test_admin_export_config():
    print_test("Admin API - Export Config")
    
    response = requests.get(f"{BASE_URL}/admin/api/config/export")
    assert response.status_code == 200, f"Failed to export config: {response.status_code}"
    assert response.headers.get('Content-Type') == 'application/x-yaml', "Wrong content type"
    assert 'config.yaml' in response.headers.get('Content-Disposition', ''), "Missing filename"
    
    config_text = response.text
    assert 'server:' in config_text, "Missing server section in config"
    assert 'queues:' in config_text, "Missing queues section in config"
    assert 'admin-test-queue' in config_text, "Queue not found in exported config"
    assert 'visibility_timeout: 60' in config_text, "Queue settings not in config"
    
    print_success("Config exported successfully")
    print_info(f"Config size: {len(config_text)} bytes")

def test_admin_delete_queue():
    print_test("Admin API - Delete Queue")
    
    response = requests.delete(f"{BASE_URL}/admin/api/queue?name=admin-test-queue")
    assert response.status_code == 200, f"Failed to delete queue via admin API: {response.text}"
    
    data = response.json()
    assert data['success'] == True, "Expected success=true"
    assert 'deleted successfully' in data['message'].lower(), f"Unexpected message: {data['message']}"
    
    print_success("Queue deleted successfully via admin API")
    
    # Verify queue is gone
    response = requests.get(f"{BASE_URL}/admin/api/queues")
    data = response.json()
    queue_names = [q['name'] for q in data['queues']]
    assert 'admin-test-queue' not in queue_names, "Queue still exists after deletion"
    
    print_success("Queue confirmed deleted")

def run_all_tests():
    print(f"\n{Colors.BLUE}{'='*60}{Colors.END}")
    print(f"{Colors.BLUE}  Ess-Queue-Ess Integration Tests{Colors.END}")
    print(f"{Colors.BLUE}{'='*60}{Colors.END}")
    
    try:
        # Basic connectivity
        test_health_check()
        test_admin_ui()
        
        # Initial state
        initial_queues = test_admin_api()
        
        # Queue operations
        queue_name = test_create_queue()
        test_list_queues(expected_count=1)
        
        # Message operations
        test_send_message(queue_name)
        test_send_multiple_messages(queue_name, count=5)
        test_receive_message(queue_name, expected_count=6)
        test_delete_message(queue_name)
        test_get_queue_attributes(queue_name)
        
        # Advanced operations
        test_purge_queue(queue_name)
        test_delete_queue(queue_name)
        
        # Admin integration
        test_admin_api_with_messages()
        
        # New admin API tests
        test_admin_create_queue()
        test_admin_send_message()
        test_admin_export_config()
        test_admin_delete_queue()
        
        print(f"\n{Colors.GREEN}{'='*60}{Colors.END}")
        print(f"{Colors.GREEN}  ✓ All tests passed!{Colors.END}")
        print(f"{Colors.GREEN}{'='*60}{Colors.END}\n")
        return 0
        
    except AssertionError as e:
        print_error(f"Test failed: {e}")
        return 1
    except requests.exceptions.ConnectionError:
        print_error("Cannot connect to Ess-Queue-Ess. Is it running on http://localhost:9324?")
        print_info("Run: docker compose up -d")
        return 1
    except Exception as e:
        print_error(f"Unexpected error: {e}")
        import traceback
        traceback.print_exc()
        return 1

if __name__ == "__main__":
    sys.exit(run_all_tests())
