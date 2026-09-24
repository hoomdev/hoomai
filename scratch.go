package main

import (
	"fmt"
	"github.com/hoomdev/hoomai/internal/providers"
)

func main() {
	p, _ := providers.Lookup("claude")
	corr, ok := p.(interface{ NewNormalizer() func(line string) []providers.Event })
	if !ok {
		fmt.Println("Not correlating")
		return
	}
	norm := corr.NewNormalizer()
	
	// Open delegation
	evs1 := norm(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_123","name":"Task","input":{"subagent_type":"test-agent"}}]}}`)
	fmt.Printf("After open length: %d\n", len(evs1))
	
	// Send running status
	evs2 := norm(`{"type":"system","subtype":"task_notification","tool_use_id":"toolu_123","status":"running"}`)
	fmt.Printf("After running length: %d\n", len(evs2))
	for _, e := range evs2 {
		fmt.Printf("Event: %+v\n", e)
	}
}
