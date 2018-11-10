package raftkv

import "labrpc"
import "math/big"
import "sync"
import "time"
import "encoding/base64"
import "fmt"
import crand "crypto/rand"

// import "sync/atomic"

type Clerk struct {
	servers []*labrpc.ClientEnd
	// You will have to modify this struct.
	mu              sync.Mutex
	leaderId        int   // latest leader the client preserves.
	opSerialNumber  int64 // current SerialNumber to uniquely identify a command RPC call.
	clientId        string
	chanReturnReady chan bool // channel for noticing reply returns.
}

const (
	WAIT_INTERVAL              = 3000
	REQUEST_INTERVAL           = 100
	DEBUG_PREFIX_FORMAT        = "%-45s"
	CLIENT_ID_LEN              = 10
	CLIENT_REQUEST_RETRY_TIMES = 32
	chanSize                   = 1
)

func nrand() int64 {
	max := big.NewInt(int64(1) << 62)
	bigx, _ := crand.Int(crand.Reader, max)
	x := bigx.Int64()
	return x
}
func srand(n int) string {
	b := make([]byte, 2*n)
	crand.Read(b)
	s := base64.URLEncoding.EncodeToString(b)
	return s[0:n]
}

func MakeClerk(servers []*labrpc.ClientEnd) *Clerk {
	ck := new(Clerk)
	ck.servers = servers
	// You'll have to add code here.
	ck.leaderId = 0
	ck.clientId = srand(CLIENT_ID_LEN)
	ck.chanReturnReady = make(chan bool, chanSize)
	fmt.Printf(DEBUG_PREFIX_FORMAT+"Client(%v) create...\n", "[Clerk-MakeClerk]", ck.clientId)
	return ck
}

//
// fetch the current value for a key.
// returns "" if the key does not exist.
// keeps trying forever in the face of all other errors.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.Get", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
//
func (ck *Clerk) Get(key string) string {
	// You will have to modify this function.
	ck.mu.Lock()
	args := GetArgs{
		Key:          key,
		SerialNumber: nrand(),
	}
	fmt.Printf(DEBUG_PREFIX_FORMAT+"Client(%v) issues a command(%v): %v.\n", "[Clerk-Get]", ck.clientId, args.SerialNumber, args)
	ck.mu.Unlock()
	i := 0
	var reply GetReply
	CommandCall := func(reply *GetReply) {
		ok := ck.servers[ck.leaderId].Call("KVServer.Get", &args, reply)
		if ok {
			ck.chanReturnReady <- true
		}
	}
	exit := false
	for i < CLIENT_REQUEST_RETRY_TIMES && !exit {
		reply := GetReply{}
		go CommandCall(&reply)
		select {
		case <-ck.chanReturnReady:
			if !reply.WrongLeader && reply.Err == "" && reply.Result == true {
				// exit = true
				fmt.Printf(DEBUG_PREFIX_FORMAT+"Command(%v) call by client(%v) result: %v \n", "[Clerk-Get]", args.SerialNumber, ck.clientId, reply.Value)
				return reply.Value
			} else {
				// ck.leaderId = rand.Intn(len(ck.servers))
				ck.mu.Lock()
				fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v) reply error for command(%v): %v\n", "[Clerk-Get]", ck.leaderId, args.SerialNumber, reply.Err)
				ck.leaderId = (ck.leaderId + 1) % len(ck.servers)
				ck.mu.Unlock()
			}
		case <-time.After(time.Duration(WAIT_INTERVAL) * time.Millisecond):
			fmt.Printf(DEBUG_PREFIX_FORMAT+"Reply of command(%v): %v timeouts by Server(%v).\n", "[Clerk-Get]", args.SerialNumber, args, ck.leaderId)
			ck.mu.Lock()
			ck.leaderId = (ck.leaderId + 1) % len(ck.servers)
			ck.mu.Unlock()
		}
		if !exit {
			fmt.Printf(DEBUG_PREFIX_FORMAT+"Client(%v) sends a command(%v): %v by redirecting  to Server(%v).\n", "[Clerk-Get]", ck.clientId, args.SerialNumber, args, ck.leaderId)
		}
		time.Sleep(REQUEST_INTERVAL * time.Millisecond)
		i++
	}
	if reply.Err != "" {
		fmt.Printf(DEBUG_PREFIX_FORMAT+"Command(%v) call by client(%v) replys error from Server(%v), %v.\n", "[Clerk-Get]", args.SerialNumber, ck.clientId, ck.leaderId, reply.Err)
		return ""
	} else {
		fmt.Printf(DEBUG_PREFIX_FORMAT+"Command(%v) call by client(%v) replys success from Server(%v).\n", "[Clerk-Get]", args.SerialNumber, ck.clientId, ck.leaderId)
		fmt.Printf(DEBUG_PREFIX_FORMAT+"Command(%v) call by client(%v) result: %v \n", "[Clerk-Get]", args.SerialNumber, ck.clientId, reply.Value)
		return reply.Value
	}
}

//
// shared by Put and Append.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.PutAppend", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
//
func (ck *Clerk) PutAppend(key string, value string, op string) {
	// You will have to modify this function.
	ck.mu.Lock()
	args := PutAppendArgs{
		Key:          key,
		Value:        value,
		Op:           op,
		SerialNumber: nrand(),
	}
	fmt.Printf(DEBUG_PREFIX_FORMAT+"Client(%v) issues a command(%v): %v.\n", "[Clerk-PutAppend]", ck.clientId, args.SerialNumber, args)
	ck.mu.Unlock()
	i := 0
	var reply PutAppendReply
	CommandCall := func(reply *PutAppendReply) {
		ok := ck.servers[ck.leaderId].Call("KVServer.PutAppend", &args, reply)
		if ok {
			ck.chanReturnReady <- true
		}
	}
	exit := false
	for i < CLIENT_REQUEST_RETRY_TIMES && !exit {
		reply := PutAppendReply{}
		go CommandCall(&reply)
		select {
		case <-ck.chanReturnReady:
			if !reply.WrongLeader && reply.Err == "" && reply.Result == true {
				// exit = true
				return
			} else {
				// ck.leaderId = rand.Intn(len(ck.servers))
				ck.mu.Lock()
				fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v) reply error for command(%v): %v\n", "[Clerk-PutAppend]", ck.leaderId, args.SerialNumber, reply.Err)
				ck.leaderId = (ck.leaderId + 1) % len(ck.servers)
				ck.mu.Unlock()
			}
		case <-time.After(time.Duration(WAIT_INTERVAL) * time.Millisecond):
			fmt.Printf(DEBUG_PREFIX_FORMAT+"Reply of command(%v): %v timeouts by Server(%v).\n", "[Clerk-PutAppend]", args.SerialNumber, args, ck.leaderId)
			// ck.leaderId = rand.Intn(len(ck.servers))
			ck.mu.Lock()
			ck.leaderId = (ck.leaderId + 1) % len(ck.servers)
			ck.mu.Unlock()
		}
		if !exit {
			fmt.Printf(DEBUG_PREFIX_FORMAT+"Client(%v) sends a command(%v): %v by redirecting  to Server(%v).\n", "[Clerk-Get]", ck.clientId, args.SerialNumber, args, ck.leaderId)
		}
		time.Sleep(REQUEST_INTERVAL * time.Millisecond)
		i++
	}
	if reply.Err != "" {
		fmt.Printf(DEBUG_PREFIX_FORMAT+"Command(%v) call by client(%v) replys error from Server(%v): %v.\n", "[Clerk-PutAppend]", args.SerialNumber, ck.clientId, ck.leaderId, reply.Err)
	} else {
		fmt.Printf(DEBUG_PREFIX_FORMAT+"Command(%v) call by client(%v) replys success from Server(%v).\n", "[Clerk-PutAppend]", args.SerialNumber, ck.clientId, ck.leaderId)
	}
	return
}

func (ck *Clerk) Put(key string, value string) {
	ck.PutAppend(key, value, "Put")
}
func (ck *Clerk) Append(key string, value string) {
	ck.PutAppend(key, value, "Append")
}
