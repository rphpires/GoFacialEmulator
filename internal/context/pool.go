package context

import (
	"context"
	"sync"
	"time"
)

var contextPool = sync.Pool{
	New: func() interface{} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		return &poolContext{ctx: ctx, cancel: cancel}
	},
}

type poolContext struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func GetContext() (*poolContext, context.CancelFunc) {
	pc := contextPool.Get().(*poolContext)
	return pc, pc.cancel
}

func PutContext(pc *poolContext) {
	pc.cancel()
	contextPool.Put(pc)
}
