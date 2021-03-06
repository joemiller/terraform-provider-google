package google

import (
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/terraform/helper/resource"
	cloudresourcemanager "google.golang.org/api/cloudresourcemanager/v1"
)

type Waiter interface {
	// State returns the current status of the operation.
	State() string

	// Error returns an error embedded in the operation we're waiting on, or nil
	// if the operation has no current error.
	Error() error

	// SetOp sets the operation we're waiting on in a Waiter struct so that it
	// can be used in other methods.
	SetOp(interface{}) error

	// QueryOp sends a request to the server to get the current status of the
	// operation.
	QueryOp() (interface{}, error)

	// OpName is the name of the operation and is used to log its status.
	OpName() string

	// PendingStates contains the values of State() that cause us to continue
	// refreshing the operation.
	PendingStates() []string

	// TargetStates contain the values of State() that cause us to finish
	// refreshing the operation.
	TargetStates() []string
}

type CommonOperationWaiter struct {
	Op CommonOperation
}

func (w *CommonOperationWaiter) State() string {
	if w == nil {
		return fmt.Sprintf("Operation is nil!")
	}

	return fmt.Sprintf("done: %v", w.Op.Done)
}

func (w *CommonOperationWaiter) Error() error {
	if w != nil && w.Op.Error != nil {
		return fmt.Errorf("Error code %v, message: %s", w.Op.Error.Code, w.Op.Error.Message)
	}
	return nil
}

func (w *CommonOperationWaiter) SetOp(op interface{}) error {
	if err := Convert(op, &w.Op); err != nil {
		return err
	}
	return nil
}

func (w *CommonOperationWaiter) OpName() string {
	if w == nil {
		return "<nil>"
	}

	return w.Op.Name
}

func (w *CommonOperationWaiter) PendingStates() []string {
	return []string{"done: false"}
}

func (w *CommonOperationWaiter) TargetStates() []string {
	return []string{"done: true"}
}

func OperationDone(w Waiter) bool {
	for _, s := range w.TargetStates() {
		if s == w.State() {
			return true
		}
	}
	return false
}

func CommonRefreshFunc(w Waiter) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		// First, read the operation from the server.
		op, err := w.QueryOp()

		// If we got a non-retryable error, return it.
		if err != nil {
			if !isRetryableError(err) {
				return nil, "", fmt.Errorf("Not retriable error: %s", err)
			}

			log.Printf("[DEBUG] Saw error polling for op, but dismissed as retriable: %s", err)
			if op == nil {
				return nil, "", fmt.Errorf("Cannot continue, Operation is nil. %s", err)
			}
		}

		log.Printf("[DEBUG] working with op %#v", op)

		// Try to set the operation (so we can check it's Error/State),
		// and fail if we can't.
		if err = w.SetOp(op); err != nil {
			return nil, "", fmt.Errorf("Cannot continue %s", err)
		}

		// Fail if the operation object contains an error.
		if err = w.Error(); err != nil {
			return nil, "", err
		}
		log.Printf("[DEBUG] Got %v while polling for operation %s's status", w.State(), w.OpName())

		return op, w.State(), nil
	}
}

func OperationWait(w Waiter, activity string, timeoutMinutes int) error {
	if OperationDone(w) {
		if w.Error() != nil {
			return w.Error()
		}
		return nil
	}

	c := &resource.StateChangeConf{
		Pending:    w.PendingStates(),
		Target:     w.TargetStates(),
		Refresh:    CommonRefreshFunc(w),
		Timeout:    time.Duration(timeoutMinutes) * time.Minute,
		MinTimeout: 2 * time.Second,
	}
	opRaw, err := c.WaitForState()
	if err != nil {
		return fmt.Errorf("Error waiting for %s: %s", activity, err)
	}

	err = w.SetOp(opRaw)
	if err != nil {
		return err
	}
	if w.Error() != nil {
		return w.Error()
	}

	return nil
}

// The cloud resource manager API operation is an example of one of many
// interchangeable API operations. Choose it somewhat arbitrarily to represent
// the "common" operation.
type CommonOperation cloudresourcemanager.Operation
