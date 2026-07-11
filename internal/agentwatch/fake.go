package agentwatch

import "github.com/matteobortolazzo/agent-stack/agentwatch/pkg/watch"

// Compile-time check: *FakeWatcher implements Watcher.
var _ Watcher = (*FakeWatcher)(nil)

// FakeWatcherResult is one scripted result returned by FakeWatcher.ReadNext.
type FakeWatcherResult struct {
	Snap *watch.StateSnapshot
	Err  error
}

// FakeWatcher is an in-memory Watcher for tests. Results are returned in
// order by successive ReadNext calls; once exhausted, ReadNext returns
// (nil, nil).
type FakeWatcher struct {
	Results []FakeWatcherResult
	index   int
	closed  bool
}

// ReadNext returns the next scripted result, in order.
func (fw *FakeWatcher) ReadNext() (*watch.StateSnapshot, error) {
	if fw.index >= len(fw.Results) {
		return nil, nil
	}
	result := fw.Results[fw.index]
	fw.index++
	return result.Snap, result.Err
}

// Close marks the watcher as closed and returns nil.
func (fw *FakeWatcher) Close() error {
	fw.closed = true
	return nil
}
