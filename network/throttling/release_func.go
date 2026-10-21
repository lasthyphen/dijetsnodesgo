

package throttling

type ReleaseFunc func()

func noopRelease() {}
