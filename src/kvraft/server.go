package raftkv

import (
	"fmt"
	"labgob"
	"labrpc"
	"log"
	"raft"
	"reflect"
	"sync"
	"time"
)

const Debug = 0

const (
	OP_PUT    = "Put"
	OP_APPEND = "Append"
	OP_GET    = "Get"

	SERVER_WAIT_INTERVAL = 1000
	SERVER_CHAN_SIZE     = 1
)

func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug > 0 {
		log.Printf(format, a...)
	}
	return
}

// Field names must start with capital letters,
// otherwise RPC will break.
type Op struct {
	// Your definitions here.
	Opcode       string   // operation type.
	Operand      []string // operation data.
	SerialNumber int64    // uniquely identify client operations to ensure execute each one just once.
}

type KVServer struct {
	mu      sync.Mutex
	me      int
	rf      *raft.Raft
	applyCh chan raft.ApplyMsg

	maxraftstate int // snapshot if log grows this big

	// Your definitions here.
	overCall         map[int]chan Op
	overCallMapMutex sync.RWMutex

	successCall    map[int64]bool
	storageService map[string]string
}

func (kv *KVServer) Get(args *GetArgs, reply *GetReply) {
	// Your code here.
	operand := []string{args.Key}
	op := Op{
		Opcode:       OP_GET,
		Operand:      operand,
		SerialNumber: args.SerialNumber,
	}
	cmdIndex, _, leader := kv.rf.Start(op)
	if !leader {
		fmt.Printf(DEBUG_PREFIX_FORMAT+"Client choose Server(%v), wrong!\n", "[Server-Get]", kv.me)
		reply.WrongLeader = true
		reply.Err = "Request host is not the leader."
		return
	}

	kv.mu.Lock()
	overCallChannel, ok := kv.overCall[cmdIndex]
	if !ok {
		overCallChannel = make(chan Op, chanSize)
		kv.overCall[cmdIndex] = overCallChannel
	}
	kv.mu.Unlock()
	select {
	case cmd := <-overCallChannel:
		if reflect.DeepEqual(cmd, op) {
			kv.mu.Lock()
			reply.WrongLeader = false
			reply.Result = true
			reply.Err = ""
			reply.Value = kv.storageService[args.Key]
			kv.successCall[args.SerialNumber] = true
			kv.mu.Unlock()
			fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v): Reply  SUCCESS from raft for command(%v).\n", "[Server-Get]", kv.me, args.SerialNumber)
			fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v): Return result for command(%v). result: key=%v, value=%v.\n", "[Server-Get]", kv.me, args.SerialNumber, args.Key, reply.Value)
			kv.mu.Lock()
			fmt.Printf("===========command(%v)==================\n", args.SerialNumber)
			fmt.Printf("%q", kv.storageService)
			fmt.Printf("\n=============================\n")
			kv.mu.Unlock()
			return
		}
	case <-time.After(time.Duration(SERVER_WAIT_INTERVAL) * time.Millisecond):
		reply.WrongLeader = true
		reply.Result = false
		reply.Err = "Request lost by raft node."
		fmt.Printf(DEBUG_PREFIX_FORMAT+" Server(%v) wait raft timeout for command(%v)!\n", "[Server-Get]", kv.me, args.SerialNumber)
		return
	}
	fmt.Printf("---------------------------------------------------1---------------------------------------------------------\n")
}

func (kv *KVServer) PutAppend(args *PutAppendArgs, reply *PutAppendReply) {
	// Your code here.
	operand := []string{args.Key, args.Value}
	op := Op{
		Opcode:       args.Op,
		Operand:      operand,
		SerialNumber: args.SerialNumber,
	}
	cmdIndex, _, leader := kv.rf.Start(op)
	if !leader {
		fmt.Printf(DEBUG_PREFIX_FORMAT+"Client choose Server(%v), wrong!\n", "[Server-PutAppend]", kv.me)
		reply.WrongLeader = true
		reply.Err = "Request host is not the leader."
		return
	}
	kv.mu.Lock()
	overCallChannel, ok := kv.overCall[cmdIndex]
	if !ok {
		overCallChannel = make(chan Op, chanSize)
		kv.overCall[cmdIndex] = overCallChannel
	}
	kv.mu.Unlock()
	select {
	case cmd := <-overCallChannel:
		if reflect.DeepEqual(cmd, op) {
			reply.WrongLeader = false
			reply.Err = ""
			reply.Result = true
			fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v): Reply SUCCESS from raft for command(%v).\n", "[Server-PutAppend]", kv.me, args.SerialNumber)
			fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v): Return result for command(%v). result: key=%v, value=%v.\n", "[Server-PutAppend]", kv.me, args.SerialNumber, args.Key, kv.storageService[args.Key])
			kv.mu.Lock()
			fmt.Printf("===========command(%v)==================\n", args.SerialNumber)
			fmt.Printf("%q", kv.storageService)
			fmt.Printf("\n=============================\n")
			kv.mu.Unlock()
			return
		}
	case <-time.After(time.Duration(SERVER_WAIT_INTERVAL) * time.Millisecond):
		reply.WrongLeader = true
		reply.Result = false
		reply.Err = "Request lost by raft node."
		fmt.Printf(DEBUG_PREFIX_FORMAT+" Server(%v) wait raft timeout for command(%v)!\n", "[Server-PutAppend]", kv.me, args.SerialNumber)
		return
	}
	fmt.Printf("------------------------------------------------2------------------------------------------------------------\n")
}

func (kv *KVServer) applyToStorage(args Op) {
	if args.Opcode == OP_PUT {
		kv.storageService[args.Operand[0]] = args.Operand[1]
	} else if args.Opcode == OP_APPEND {
		kv.storageService[args.Operand[0]] += args.Operand[1]
	}
	kv.successCall[args.SerialNumber] = true
}

func (kv *KVServer) handleApplyMsg() {
	for {
		select {
		case m := <-kv.applyCh:
			if m.CommandValid == false {
				fmt.Printf(DEBUG_PREFIX_FORMAT+"Code should not ======[1]===== reach here.\n", "[Server-HandleApplyMsg]")
			} else if cmd, ok := (m.Command).(Op); ok {
				kv.mu.Lock()
				// v, ok1 := kv.successCall[cmd.SerialNumber]
				// if !ok1 && !v {
				// 	kv.applyToStorage(cmd)
				// } else {
				// 	fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v) command(%v) exists in Map successCall!.\n", "[Server-HandleApplyMsg]", kv.me, cmd.SerialNumber)
				// }
				_, ok1 := kv.successCall[cmd.SerialNumber]
				if !ok1 {
					kv.applyToStorage(cmd)
				} else {
					fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v) command(%v) exists in Map successCall!.\n", "[Server-HandleApplyMsg]", kv.me, cmd.SerialNumber)
				}
				ch, ok2 := kv.overCall[m.CommandIndex]
				if ok2 {
					select {
					case <-kv.overCall[m.CommandIndex]:
					default:
					}
					ch <- cmd
				} else {
					kv.overCall[m.CommandIndex] = make(chan Op, chanSize)
				}
				kv.mu.Unlock()
			} else {
				fmt.Printf(DEBUG_PREFIX_FORMAT+"Code should not ======[2]===== reach here.\n", "[Server-HandleApplyMsg]")
			}
		}
	}
}

//
// the tester calls Kill() when a KVServer instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (kv *KVServer) Kill() {
	kv.rf.Kill()
	// Your code here, if desired.
	fmt.Sprintf(DEBUG_PREFIX_FORMAT+"Kv Server(%v) is exiting now.\n", "[Server-Kill]", kv.me)
}

//
// servers[] contains the ports of the set of
// servers that will cooperate via Raft to
// form the fault-tolerant key/value service.
// me is the index of the current server in servers[].
// the k/v server should store snapshots through the underlying Raft
// implementation, which should call persister.SaveStateAndSnapshot() to
// atomically save the Raft state along with the snapshot.
// the k/v server should snapshot when Raft's saved state exceeds maxraftstate bytes,
// in order to allow Raft to garbage-collect its log. if maxraftstate is -1,
// you don't need to snapshot.
// StartKVServer() must return quickly, so it should start goroutines
// for any long-running work.
//
func StartKVServer(servers []*labrpc.ClientEnd, me int, persister *raft.Persister, maxraftstate int) *KVServer {
	// call labgob.Register on structures you want
	// Go's RPC library to marshall/unmarshall.
	labgob.Register(Op{})

	kv := new(KVServer)
	kv.me = me
	kv.maxraftstate = maxraftstate

	fmt.Sprintf(DEBUG_PREFIX_FORMAT+"Kv Server(%v) is starting now.\n", "[Server-StartKVServer]", kv.me)
	// You may need initialization code here.
	kv.overCall = make(map[int]chan Op)
	kv.overCallMapMutex = sync.RWMutex{}
	kv.storageService = make(map[string]string)
	kv.successCall = make(map[int64]bool)

	kv.applyCh = make(chan raft.ApplyMsg)
	kv.rf = raft.Make(servers, me, persister, kv.applyCh)

	// You may need initialization code here.
	// a goroutine to handle apply message by channel.
	go kv.handleApplyMsg()

	return kv
}
