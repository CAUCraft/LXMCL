package config

import "math"

// Keep the saved high-water mark even after instances are removed. Older files
// may omit it or contain a stale value; existing IDs always establish a floor.
// MaxInt is an exhausted marker, avoiding overflow in both migration and Create.
func (s State) nextInstanceID() int {
	next := max(1, s.NextID)
	for _, i := range s.Instances {
		if i.ID == math.MaxInt {
			return math.MaxInt
		}
		next = max(next, i.ID+1)
	}
	return next
}
