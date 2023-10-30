package mgmapi

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	klog "k8s.io/klog/v2"

	"github.com/mysql/ndb-operator/config/debug"
)

// Event holds the information sent by the server
type Event struct {
	// Note : when extending this struct, update the
	// eventMemberSetters map to handle storage of the new members.
	Type         EventType
	Category     EventCategory
	SourceNodeId uint64
	// Data stores all the info related to the event type.
	// Note that all the values sent by the management server at the
	// time of writing this are unsigned integers. If the wire protocol
	// changes in the future, the type of value needs to be updated and
	// handled accordingly.
	Data map[string]uint64
}

// eventMemberSetters maps the event reply key to functions that know how
// and where to store their value inside an Event object.
// Note : Add new setter methods here when the Event struct is extended
var eventMemberSetters = map[string]func(e *Event, value uint64){

	"type": func(e *Event, value uint64) {
		e.Type = EventType(value)
		e.Category = e.Type.GetEventCategory()
	},

	"source_nodeid": func(e *Event, value uint64) {
		e.SourceNodeId = value
	},
}

// set extracts the key and value from the given string
// and updates them in the appropriate fields of the Event.
func (e *Event) set(keyValueToken string) error {
	// extract the information
	tokens := strings.SplitN(keyValueToken, "=", 2)
	if len(tokens) != 2 {
		// The reply detail is not of form key:value.
		// Usage issue or MySQL Cluster has updated its wire protocol.
		// In any case, panic in debug builds to bring in early attention.
		klog.Error("reply has unexpected format : ", keyValueToken)
		return debug.InternalError("missing '=' in log event detail")
	}

	// All the values at the point of writing this code are of type unsigned int.
	// Convert the values accordingly and verify that assumption.
	value, err := strconv.ParseUint(tokens[1], 10, 64)
	if err != nil {
		// The assumption that all values are unsigned integers has failed.
		// Need to update this code and Data member in Event struct.
		return debug.InternalError("failed to parse value of " + tokens[0] + " : " + err.Error())
	}

	// set the value appropriately
	if memberSetter, exists := eventMemberSetters[tokens[0]]; exists {
		memberSetter(e, value)
	} else {
		// setter doesn't exist - add it as a generic Data
		e.Data[tokens[0]] = value
	}

	return nil
}

func (e *Event) String() string {
	var result string
	result += "Type : " + e.Type.String() + "\n"
	result += "Category : " + e.Category.String() + "\n"
	result += "Source Node ID : " + fmt.Sprintf("%d", e.SourceNodeId) + "\n"
	// print all the extra info
	for key, value := range e.Data {
		result += fmt.Sprintf("%s : %d\n", key, value)
	}
	return result
}

// The NdbLogEventHandle implementation in Go
type NdbLogEventHandle interface {
	Destroy()
	NextEvent(eventTypeFilters ...EventType) (*Event, error)
	RegisterEventCallback(callback func(*Event) error, eventTypes ...EventType) chan error
}

type ndbLogEventHandleImpl struct {
	// connection to the management server through which the events are sent
	connection net.Conn
	// mutex to protect access to connection and the waitgroup
	connectionMutex sync.Mutex
	// waitgroup to protect the connection from getting destroyed when it is being accessed
	connectionWaitGroup sync.WaitGroup
	// closeConnection is the cancelFunc of the cancellable context used by
	// the NdbLogEventHandle. Cancelling the context will close the connection.
	closeConnection context.CancelFunc
}

// mgmMaxLogLevel is the max log level for events.
// This is used as the default for all categories.
const mgmMaxLogLevel = 15

// CreateNdbLogEventHandle creates and returns a new NdbLogEventHandle
func CreateNdbLogEventHandle(ctx context.Context, connectstring string,
	eventCategoryFilters ...EventCategory) (NdbLogEventHandle, error) {

	if len(eventCategoryFilters) == 0 {
		// empty filter is not allowed
		return nil, debug.InternalError("filter cannot be empty")
	}

	// The NdbLogEventHandle is just another mgmapi client initialised with
	// the command "listen event". So, first create an mgm client.
	mci := &mgmClientImpl{}
	err := mci.connect(connectstring)
	if err != nil {
		klog.Errorf("Failed to create NdbLogEventHandle : %s", err)
		return nil, err
	}
	defer mci.Disconnect()

	// Now that we have connected to the management server, execute the 'listen event' command
	// command :
	// listen event
	// filter: <list of event category and levels>
	// parsable: <1|0>
	// node: <nodeId - optional and ignored by the server>

	// reply :
	// listen event
	// result: 0
	// msg: ""

	// build filter in the form 'cat=level cat=level...'
	// we always use mgmMaxLogLevel for level to get all the events
	sep := ""
	var filterStr string
	for _, cat := range eventCategoryFilters {
		filterStr += sep + fmt.Sprintf("%d=%d", cat, mgmMaxLogLevel)
		sep = " "
	}

	// build args
	args := map[string]interface{}{
		"filter":   filterStr,
		"parsable": 1,
	}

	// send the command and read the reply
	reply, err := mci.executeCommand(
		"listen event", args, false,
		[]string{"listen event", "result"})
	if err != nil {
		return nil, err
	}

	// check for errors in reply
	if result, _ := strconv.Atoi(reply["result"]); result != 0 {
		return nil, debug.InternalError("CreateNdbLogEventHandle failed :" + reply["msg"])
	}

	// command succeeded - create a new NdbLogEventHandle that uses the new mgm client's connection
	le := &ndbLogEventHandleImpl{connection: mci.connection}

	// set mci.connection to nil to prevent defer Disconnect from closing it
	mci.connection = nil

	// start a go routine to clean up connection when the ctx is done or cancelled
	leCtx, cancelFunc := context.WithCancel(ctx)
	go func() {
		select {
		case <-leCtx.Done():
			// ctx is done - close the connection
			le.connectionWaitGroup.Wait()
			le.connectionMutex.Lock()
			defer le.connectionMutex.Unlock()
			if le.connection != nil {
				_ = le.connection.Close()
				le.connection = nil
				klog.V(4).Infof("ndbLogEventHandleImpl : Management server disconnected.")
			}
		}
	}()
	// store the cancel function in the handle so that it can be used later to close the connection
	le.closeConnection = cancelFunc

	return le, nil
}

// Destroy closes the connection to the Management Server
func (logEventHandle *ndbLogEventHandleImpl) Destroy() {
	// cancel the context to close the connection
	logEventHandle.closeConnection()
}

// NextEvent reads and parses event log until it receives an event matching the specified event types in eventTypeFilters
// and returns that event. If no eventTypeFilters are provided, NextEvent returns the first event it encounters.
func (logEventHandle *ndbLogEventHandleImpl) NextEvent(eventTypeFilters ...EventType) (*Event, error) {
	// build a map out of of eventTypeFilters
	filter := make(map[EventType]interface{})
	for _, eventType := range eventTypeFilters {
		filter[eventType] = nil
	}

	// lock the connection to ensure only one client can listen via this method at any given time
	logEventHandle.connectionMutex.Lock()
	defer logEventHandle.connectionMutex.Unlock()

	if logEventHandle.connection == nil {
		// connection is closed already
		return nil, debug.InternalError("Connection to Management Server has been closed. Use a new Handle")
	}

	// Start reading the reply via a scanner
	scanner := bufio.NewScanner(logEventHandle.connection)

	var event *Event
	for scanner.Scan() {
		// Set read deadline for every loop.
		// The event log constantly gets sent a '<PING>' message - almost two every second.
		// So, 5s is a very reasonable time out duration.
		logEventHandle.connection.SetReadDeadline(time.Now().Add(5 * time.Second))

		// read the line
		replyLine := scanner.Text()

		switch replyLine {
		case "<PING>":
			// the mgm server keeps sending <PING> messages
			// to check the connection - ignore them

		case eventHeader:
			// start of an event.
			// verify that the previous event has been read fully.
			if event != nil {
				// previous event has not been closed yet
				// this is a dev bug - panic in debug builds
				return nil, debug.InternalError("encoutered new event when already reading another event")
			}
			// initialize the new event to start reading
			event = &Event{Data: make(map[string]uint64)}

		case eventFooter:
			// end of event
			if event == nil {
				// no event was being read
				// this is a dev bug - panic in debug builds
				return nil, debug.InternalError("encoutered end of event but no event is being read")
			}

			// verify the key fields have been read
			if event.Type == 0 || event.SourceNodeId == 0 {
				// event has not been read fully
				return nil, debug.InternalError("received an incomplete event")
			}

			if eventTypeFilters == nil {
				// no eventTypeFilters passed - return the first event encountered
				return event, nil
			}

			if _, exists := filter[event.Type]; exists {
				// the event type exists in eventTypeFilters
				return event, nil
			}

			// caller is not interested in this event - ignore it
			event = nil
		default:
			// event is being read
			if event == nil {
				// ... but event is nil
				// this is a dev bug - panic in debug builds
				return nil, debug.InternalError("unable to parse event as no event is being read")
			}

			if err := event.set(replyLine); err != nil {
				return nil, err
			}
		}
	}

	// Control reaches here only on an scanner error
	var err error
	if err = scanner.Err(); err == nil {
		// no error => scanner reached EOF, which cannot
		// happen under normal circumstances - treat this as an error.
		err = fmt.Errorf("NdbLogEventHandle connection was closed")
	}

	klog.Error("failed to read event log from Management Server :", err)
	return nil, err
}

// RegisterEventCallback registers a callback to be executed once the event
// log encounters an event with one of the types mentioned in the eventTypes.
// If eventTypes is empty, the callback is executed after encountering the
// first event regardless of its type.
// Returns a error channel which can be consumed to check for errors that
// occured during the callback execution.
//
// For example, to retrigger reconciliation after Backup has completed,
// register a callback function that annotates the Ndb object with the
// latest backup id to retrigger the reconciliation or even directly adds
// the Ndb object key to the controller's workqueue.
func (logEventHandle *ndbLogEventHandleImpl) RegisterEventCallback(
	callback func(*Event) error, eventTypes ...EventType) chan error {
	callbackError := make(chan error, 1)

	// Add to waitgroup to prevent the connection from getting destroyed when a client is using it
	// The Go routine itself is protected by the mutex locks acquired inside NextEvent
	logEventHandle.connectionWaitGroup.Add(1)

	// start a go routine
	go func() {
		event, err := logEventHandle.NextEvent(eventTypes...)
		if err != nil {
			klog.Errorf("RegisterEventCallback failed to get NextEvent : %v", err)
			callbackError <- err
			return
		}

		// execute the callback and send back the error via error channel
		callbackError <- callback(event)

		// Mark done in waitGroup to allow connection to be closed
		logEventHandle.connectionWaitGroup.Done()
	}()

	return callbackError
}

// RegisterEventCallback is a wrapper around NdbLogEventHandle's RegisterEventCallback that
// can be called directly without initialising a NdbLogEventHandle separately.
func RegisterEventCallback(ctx context.Context, connectstring string,
	callback func(*Event) error, eventTypes ...EventType) (error, chan error) {

	// deduce all the required event categories that need to be filtered
	var eventCategoryFilters []EventCategory
	for _, eventType := range eventTypes {
		eventCategoryFilters = append(eventCategoryFilters, eventType.GetEventCategory())
	}

	// create a new logEventHandle
	logEventHandle, err := CreateNdbLogEventHandle(ctx, connectstring, eventCategoryFilters...)
	if err != nil {
		return err, nil
	}
	// setup defer to destroy the handle.
	// although Destroy is called at the end of this method, access to the connection
	// is protected by the mutex and it will be closed only after the callback is executed.
	defer logEventHandle.Destroy()

	// register the callback and return the error channel
	return nil, logEventHandle.RegisterEventCallback(callback, eventTypes...)
}
