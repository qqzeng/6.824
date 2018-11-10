package mapreduce

import (
	"fmt"
	"sync"
)

//
// schedule() starts and waits for all tasks in the given phase (mapPhase
// or reducePhase). the mapFiles argument holds the names of the files that
// are the inputs to the map phase, one per map task. nReduce is the
// number of reduce tasks. the registerChan argument yields a stream
// of registered workers; each item is the worker's RPC address,
// suitable for passing to call(). registerChan will yield all
// existing registered workers (if any) and new ones as they register.
//
func schedule(jobName string, mapFiles []string, nReduce int, phase jobPhase, registerChan chan string) {
	var ntasks int
	var n_other int // number of inputs (for reduce) or outputs (for map)
	switch phase {
	case mapPhase:
		ntasks = len(mapFiles)
		n_other = nReduce
	case reducePhase:
		ntasks = nReduce
		n_other = len(mapFiles)
	}

	fmt.Printf("Schedule: %v %v tasks (%d I/Os)\n", ntasks, phase, n_other)

	// All ntasks tasks have to be scheduled on workers. Once all tasks
	// have completed successfully, schedule() should return.
	//
	// Your code here (Part III, Part IV).
	//
	var wg sync.WaitGroup
	for i := 0; i < ntasks; i++ {
		wg.Add(1)
		go func(taskIndex int) {
			defer wg.Done()
			// 1. Build DoTaskArgs entity.
			var args DoTaskArgs
			args.Phase = phase
			args.JobName = jobName
			args.TaskNumber = taskIndex
			args.NumOtherPhase = n_other
			if phase == mapPhase {
				args.File = mapFiles[taskIndex]
			}
			// 2. Send DoTaskJob rpc.
			for { // Note Loop call util success if there exists worker failure.
				// 3. Fetch worker from registerChan.
				// Note if one worker goes down, it won't be fetched from registerChan.
				work := <-registerChan
				ok := call(work, "Worker.DoTask", &args, new(struct{}))
				if ok == false {
					fmt.Printf("Master: RPC %s schedule error for %s task %d.\n", work, phase, taskIndex)
					fmt.Printf("Master: Redo - RPC schedule error for %s task %d.\n", phase, taskIndex)
				} else {
					fmt.Printf("Master: RPC %s schedule success for %s task %d.\n", work, phase, taskIndex)
					// 4. Register worker (ready) in parallel to avoid block.
					go func() {
						registerChan <- work
					}()
					fmt.Printf("Master: %d tasks over!\n", taskIndex)
					break
				}
			}
		}(i)
	}
	wg.Wait() // Wait for all (ntasks) task to compelete.
	fmt.Printf("Schedule: %v done\n", phase)
}
