package raftkv

const (
	OK       = "OK"
	ErrNoKey = "ErrNoKey"
)

type Err string

// Put or Append
type PutAppendArgs struct {
	Key   string
	Value string
	Op    string // "Put" or "Append"
	// You'll have to add definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
	SerialNumber int64 // uniquely identify client operation to ensure execute each one just once.
}

type PutAppendReply struct {
	WrongLeader bool
	Err         Err
	Result      bool
}

type GetArgs struct {
	Key string
	// You'll have to add definitions here.
	SerialNumber int64 // uniquely identify client operation to ensure execute each one just once.
}

type GetReply struct {
	WrongLeader bool
	Err         Err
	Value       string
	Result      bool
}
