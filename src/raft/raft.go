package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	"bytes"
	"fmt"
	"labgob"
	"labrpc"
	"log"
	"math/rand"
	"sync"
	"time"
)

//
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
//
// in Lab 3 you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh; at that point you can add fields to
// ApplyMsg, but set CommandValid to false for these other uses.
//
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int
}

type NodeState string

const (
	Follower  NodeState = "Follower"
	Candidate NodeState = "Candidate"
	Leader    NodeState = "Leader"
)

const (
	HeartBeatInterval    = 100 * time.Millisecond
	ElectionTimeOutBase  = 400
	ElectionTimeOutRange = 300

	chanSize = 1

	// Log Entry base(starting) log index. expected 1 to pass test2B.
	BASE_LOG_INDEX      = 1
	DEBUG_PREFIX_FORMAT = "%-45s"
)

// LogEntry object.
type LogEntry struct {
	LogIndex int
	Term     int
	Command  interface{}
}

//
// A Go object implementing a single Raft peer.
//
type Raft struct {
	// Note: In our implementation, it's crucial or else the [TestReElection2A] will fail occasionally if type command `go test -run 2A [> out]`.
	alive bool // Indicate whether a raft node should continue to work.

	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]

	// Your data here (2A, 2B, 2C).
	// general state
	state NodeState // this peer is a follower, a candidate or a leader?

	// Updated on stable storage before responding to RPCs
	currentTerm int // latest term the peer has seen
	votedFor    int // candidateId(peer's index) that received vote in current term (or null if none).
	votedCount  int // the count of voting for this node.

	logs []LogEntry // log entries.

	// volatile state.
	commitIndex int // index of highest log entry known to be committed.
	lastApplied int // index of highest log entry applied to state machine.

	// volatile state leader holds only.
	// Reinitialized after election.
	nextIndex  []int // for each server, index of the next log entry to send to that server.
	matchIndex []int // for each server, index of highest log entry known to be replicated on server.

	// channels
	chanApply       chan ApplyMsg // channel for apply logs
	chanCommit      chan bool     // channel for commit logs
	chanHeartbeat   chan bool     // AppendEntries RPC (heartbeat) received channel.
	chanBeLeader    chan bool     // revecive majorty of votes hence step forward.
	chanRequestVote chan bool     // ReqeustVote RPC received channel.
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {

	var term int
	var isleader bool
	// Your code here (2A).
	rf.mu.Lock()
	term = rf.currentTerm
	isleader = rf.state == Leader
	rf.mu.Unlock()

	return term, isleader
}

// for each server, index of the last log entry, which is based on baseLogIndex.
func (rf *Raft) getLastLogIndex() int {
	if len(rf.logs) == BASE_LOG_INDEX {
		return BASE_LOG_INDEX - 1
	}
	return rf.logs[len(rf.logs)-1].LogIndex
}

func (rf *Raft) getLastLogTerm() int {
	if len(rf.logs) == BASE_LOG_INDEX {
		return -1
	}
	return rf.logs[len(rf.logs)-1].Term
}

func (rf *Raft) getBaseLogIndex() int {
	if len(rf.logs) == BASE_LOG_INDEX {
		return BASE_LOG_INDEX - 1
	}
	return rf.logs[BASE_LOG_INDEX].LogIndex
}

func (rf *Raft) printLogEntries(logEntries []LogEntry) {
	// rf.mu.Lock()
	// defer rf.mu.Unlock()
	if len(logEntries) > 0 {
		fmt.Printf("length=%v\t", len(logEntries))
	}
	for i := 0; i < len(logEntries); i++ {
		fmt.Printf("log(%v, %v, %v)\t", logEntries[i].LogIndex, logEntries[i].Term, logEntries[i].Command)
	}
	fmt.Printf("\n")
}

//
// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
//
func (rf *Raft) persist() {
	// Your code here (2C).
	// Example:
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(rf.currentTerm)
	e.Encode(rf.votedFor)
	e.Encode(rf.logs)
	data := w.Bytes()
	rf.persister.SaveRaftState(data)
	DPrintf(DEBUG_PREFIX_FORMAT+"Save Raft's persistent state to stable storage successfully!\n", "[persist]")
}

//
// restore previously persisted state.
//
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (2C).
	// Example:
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	var ct int
	var vf int
	var logs []LogEntry
	if d.Decode(&ct) != nil ||
		d.Decode(&vf) != nil ||
		d.Decode(&logs) != nil {
		log.Fatalf("Error to read previously persisted state!")
	} else {
		rf.currentTerm = ct
		rf.votedFor = vf
		rf.logs = logs
		DPrintf(DEBUG_PREFIX_FORMAT+"Read previously persisted state successfully!\n", "[readPersist]")
	}
}

// AppendEntries RPC struct
type AppendEntriesArgs struct {
	AEterm         int // leader's term
	AEleaderId     int // ledaer's id for follower redictes client requests
	AEprevLogIndex int // index of leader last log entry
	AEprevLogTerm  int // term of leader last log entry

	AEleaderCommit int        // leader's commitIndex.
	AEentries      []LogEntry // log entries
}

type AppendEntriesReply struct {
	AERterm      int  // currentTerm, for leader to update itself
	AERsuccess   bool // true if follower contained entry matching prevLogIndex and prevLogTerm
	AERnextIndex int  // log entry conflict occurs, follower returns its first matching log index.
}

//
// example RequestVote RPC arguments structure.
// field names must start with capital letters!
//
type RequestVoteArgs struct {
	// Your data here (2A, 2B).
	RVterm         int // candidate's term
	RVcandidateId  int // candidate requesting vote
	RVlastLogIndex int // index of candidate’s last log entry
	RVlastLogTerm  int // term of candidate’s last log entry
}

//
// example RequestVote RPC reply structure.
// field names must start with capital letters!
//
type RequestVoteReply struct {
	// Your data here (2A).
	RVRterm        int  // currentTerm, for candidate to update itself
	RVRvoteGranted bool // true means candidate received vote
}

//
// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
//
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	index := -1
	term := rf.currentTerm
	isLeader := rf.state == Leader

	// Your code here (2B).
	if isLeader {
		// NOTE: this implementation differs from paper mentions in Fig 2..
		// store the log into leader's local log entries.
		// and return stored index immediately.
		// it will be replicated to other peers in next AppendEntries RPCs.
		index = rf.getLastLogIndex() + 1
		logEntry := LogEntry{
			LogIndex: index,
			Command:  command,
			Term:     rf.currentTerm,
		}
		rf.logs = append(rf.logs, logEntry)
		rf.persist()
		fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) appends log(%v, %v).\n", "[#2B Start]", rf.state, rf.me, rf.currentTerm, index, command)
	}
	return index, term, isLeader
}

//
// the tester calls Kill() when a Raft instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (rf *Raft) Kill() {
	// Your code here, if desired.
	fmt.Printf(DEBUG_PREFIX_FORMAT+"Clear all channels.\n", "[Kill]")
	// be sure all channles release.
	for i := 0; i < len(rf.chanApply); i++ {
		<-rf.chanApply
	}
	for i := 0; i < len(rf.chanBeLeader); i++ {
		<-rf.chanBeLeader
	}
	for i := 0; i < len(rf.chanHeartbeat); i++ {
		<-rf.chanHeartbeat
	}
	for i := 0; i < len(rf.chanRequestVote); i++ {
		<-rf.chanRequestVote
	}
	for i := 0; i < len(rf.chanCommit); i++ {
		<-rf.chanCommit
	}
	rf.alive = false
	DPrintf(DEBUG_PREFIX_FORMAT+"Raft node server(%v, %v, %v) will be set to be dead.\n", "[Kill]", rf.state, rf.me, rf.currentTerm)
}

//
// example RequestVote RPC handler.
//
func (rf *Raft) RequestVoteHandler(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	defer rf.persist()

	b1 := args.RVterm < rf.currentTerm
	b2 := args.RVterm > rf.currentTerm

	if b1 == true {
		reply.RVRterm = rf.currentTerm
		fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) rejects a (outdated) RequestVote RPC from server(%v, %v).\n", "[Leader Election-RequestVoteHandler]", rf.state, rf.me, rf.currentTerm, args.RVcandidateId, args.RVterm)
		return
	}

	if b2 == true {
		fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) will update its states by RequestVote RPC from server(%v, %v).\n", "[Leader Election-RequestVoteHandler]", rf.state, rf.me, rf.currentTerm, args.RVcandidateId, args.RVterm)
		rf.currentTerm = args.RVterm
		rf.state = Follower
		rf.votedFor = -1
		rf.votedCount = 0
		fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) updates its states by RequestVote RPC from server(%v, %v).\n", "[Leader Election-RequestVoteHandler]", rf.state, rf.me, rf.currentTerm, args.RVcandidateId, args.RVterm)
		// return
	}
	reply.RVRterm = rf.currentTerm

	b3 := rf.votedFor == -1 || rf.votedFor == args.RVcandidateId

	if args.RVlastLogIndex == BASE_LOG_INDEX-1 && args.RVlastLogTerm == -1 && len(rf.logs) == BASE_LOG_INDEX {
		if b3 {
			reply.RVRvoteGranted = true
			rf.votedFor = args.RVcandidateId
			rf.chanRequestVote <- true
			DPrintf(DEBUG_PREFIX_FORMAT+"Server(%v, %v) receives vote from server(%v, %v, %v) in initial state.\n", "[Leader Election-RequestVoteHandler]", args.RVcandidateId, args.RVterm, rf.state, rf.me, rf.currentTerm)
			return
		} else {
			fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v, %v) is rejected voting from server(%v, %v, %v) in initial state.\n", "[Leader Election-RequestVoteHandler]", args.RVcandidateId, args.RVterm, rf.state, rf.me, rf.currentTerm)
			return
		}
	}

	b4 := args.RVlastLogTerm >= rf.logs[rf.getLastLogIndex()].Term && args.RVlastLogIndex >= rf.getLastLogIndex()
	b5 := args.RVlastLogTerm > rf.logs[rf.getLastLogIndex()].Term
	if b3 && (b4 || b5) {
		reply.RVRvoteGranted = true
		rf.votedFor = args.RVcandidateId
		rf.state = Follower
		rf.chanRequestVote <- true
		fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) is asked to vote from server(%v, %v).\n", "[Leader Election-RequestVoteHandler]", rf.state, rf.me, rf.currentTerm, args.RVcandidateId, args.RVterm)
		return
	}

	// already voted for other.
	if rf.votedFor != -1 {
		fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) has already voted for server(%v).\n", "[Leader Election-RequestVoteHandler]", rf.state, rf.me, rf.currentTerm, rf.votedFor)
	} else if !b5 && !b4 {
		// fmt.Printf("=======================================\n")
		// fmt.Printf("args.RVlastLogTerm=%v, args.RVlastLogIndex=%v, rf.logs[rf.getLastLogIndex()].Term=%v, rf.getLastLogIndex()=%v\n", args.RVlastLogTerm, args.RVlastLogIndex, rf.logs[rf.getLastLogIndex()].Term, rf.getLastLogIndex())
		// fmt.Printf("=======================================\n")
		fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v)'s logs  is newer than server(%v)'s.\n", "[Leader Election-RequestVoteHandler]", rf.state, rf.me, rf.currentTerm, args.RVcandidateId)
	}
	return

}

// a candidate send RequestVote RPC to a specific peer.
func (rf *Raft) sendRequestVote(p int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	DPrintf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) sends RequestVote to server(%v).\n", "[Leader Election-sendRequestVote]", rf.state, rf.me, rf.currentTerm, p)
	ok := rf.peers[p].Call("Raft.RequestVoteHandler", args, reply)

	rf.mu.Lock()
	defer rf.mu.Unlock()
	defer rf.persist()
	if !ok {
		return ok
	}

	if rf.state != Candidate {
		return ok
	}

	if reply.RVRterm > args.RVterm {
		rf.state = Follower
		rf.currentTerm = reply.RVRterm
		rf.votedFor = -1
		rf.votedCount = 0
		DPrintf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) converts to follower, because of RequestVote reply from server(%v, %v).\n", "[Leader Election-sendRequestVote]", rf.state, rf.me, rf.currentTerm, p, reply.RVRterm)
		return ok
	}

	// reply.RVRterm (<)= args.RVterm
	if reply.RVRvoteGranted {
		rf.votedCount++
		fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) receives vote from server(%v, %v), current votes is %v.\n", "[Leader Election-sendRequestVote]", rf.state, rf.me, rf.currentTerm, p, reply.RVRterm, rf.votedCount)
		if rf.state == Candidate && rf.votedCount > len(rf.peers)/2 {
			rf.state = Follower
			rf.chanBeLeader <- true
			// rf.state = Leader
			// rf.votedFor = -1
			// rf.votedCount = 0
			fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) steps forward to the leader successfully!\n", "[Leader Election-sendRequestVote]", rf.state, rf.me, rf.currentTerm)
		}
	}
	return ok
}

// a candidate send RequestVote to all peers in parallel to become a leader.
func (rf *Raft) broadcastRequestVote() {
	rf.mu.Lock()
	fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) begins broadcast RequestVote RPCs to all peers.\n", "[Leader Election-broadcastRequestVote]", rf.state, rf.me, rf.currentTerm)
	if rf.state != Candidate {
		fmt.Printf(DEBUG_PREFIX_FORMAT+"Erorr to exit: Only candidate can issue RequestVote RPCs, state is %v instead.", "[Leader Election-broadcastRequestVote]", rf.state)
		return
	}

	lastLogIndex := rf.getLastLogIndex()
	lastLogTerm := rf.getLastLogTerm()
	args := &RequestVoteArgs{
		RVterm:         rf.currentTerm,
		RVcandidateId:  rf.me,
		RVlastLogIndex: lastLogIndex,
		RVlastLogTerm:  lastLogTerm,
	}
	rf.mu.Unlock()

	for i := 0; i < len(rf.peers); i++ {
		if i != rf.me {
			go func(j int) {
				var reply RequestVoteReply
				rf.sendRequestVote(j, args, &reply)
			}(i)
		}
	}
}

// actions followers take when they recevice AppendEntries RPCs from the leader.
func (rf *Raft) AppendEntriesHandler(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	// fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) receive a AppendEntries RPC from server(%v, %v).\n", "[Append Entries-AppendEntriesHandler]", rf.state, rf.me, rf.currentTerm, args.AEleaderId, args.AEterm)
	rf.mu.Lock()
	defer rf.mu.Unlock()
	defer rf.persist()

	reply.AERsuccess = false
	reply.AERnextIndex = rf.getLastLogIndex() + 1

	if args.AEterm < rf.currentTerm {
		reply.AERterm = rf.currentTerm
		DPrintf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) receive a outdated AppendEntries RPC from server(%v, %v).\n", "[Append Entries-AppendEntriesHandler]", rf.state, rf.me, rf.currentTerm, args.AEleaderId, args.AEterm)
		return
	}

	// reset timing.
	rf.chanHeartbeat <- true

	if args.AEterm > rf.currentTerm {
		rf.currentTerm = args.AEterm
		rf.state = Follower
		rf.votedFor = -1
		rf.votedCount = 0
		fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) update its states by AppendEntries RPC from server(%v, %v).\n", "[Append Entries-AppendEntriesHandler]", rf.state, rf.me, rf.currentTerm, args.AEleaderId, args.AEterm)
	}

	reply.AERterm = rf.currentTerm

	baseLogIndex := rf.getBaseLogIndex()
	lastLogIndex := rf.getLastLogIndex()

	// NOTE: we treat this situation as that the follower has the entry with a different term.
	if args.AEprevLogIndex > lastLogIndex {
		return
	}
	// NOTE: convets its state to Follower. (in case of being Candidate)
	rf.state = Follower
	rf.votedFor = -1
	rf.votedCount = 0

	// Initial state, and no log entries replicated previously.
	// fmt.Printf("===================args.AEprevLogIndex=%v, args.AEprevLogTerm=%v, len(rf.logs)=%v===============\n", args.AEprevLogIndex, args.AEprevLogTerm, len(rf.logs))
	if args.AEprevLogIndex == BASE_LOG_INDEX-1 && args.AEprevLogTerm == -1 && baseLogIndex == BASE_LOG_INDEX-1 {
		if len(args.AEentries) > 0 {
			DPrintf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) appends logs[%v, %v] from server(%v, %v) in initial state.\n", "[#2B Append Entries-AppendEntriesHandler]", rf.state, rf.me, rf.currentTerm, BASE_LOG_INDEX, len(args.AEentries), args.AEleaderId, args.AEterm)
		} else {
			DPrintf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) sends AppendEntries RPC back to server(%v, %v) in initial state.\n", "[Append Entries-AppendEntriesHandler]", rf.state, rf.me, rf.currentTerm, args.AEleaderId, args.AEterm)
		}
	}

	// if exists conflicting entries, then overwrite all conflicts.
	if args.AEprevLogTerm != -1 {
		initialEntry := true
		tmpTerm := rf.logs[args.AEprevLogIndex].Term
		if args.AEprevLogTerm != tmpTerm {
			if args.AEprevLogIndex > baseLogIndex {
				// no optimization for conflicting entries, check one by one.
				// reply.AERnextIndex = args.AEprevLogIndex
				// optimize to reduce number of RPCs.
				for i := args.AEprevLogIndex - 1; i >= baseLogIndex; i-- {
					if rf.logs[i].Term != tmpTerm {
						reply.AERnextIndex = i + 1
						initialEntry = false
						break
					}
				}
			} else {
				reply.AERnextIndex = baseLogIndex + 1
			}
			if initialEntry {
				reply.AERnextIndex = baseLogIndex + 1
			}
			rf.logs = rf.logs[:reply.AERnextIndex]
			return
		}
	}
	// check whether the follower holds extra log entries. (may arisen from outdated AppednEntries RPC from the leader.)
	var conflict bool = false
	if len(args.AEentries) > 0 && args.AEprevLogTerm != -1 {
		for i := args.AEprevLogIndex + 1; i < len(rf.logs) && i < args.AEprevLogIndex+len(args.AEentries); i++ {
			if rf.logs[i].Term != args.AEentries[i-args.AEprevLogIndex-1].Term {
				conflict = true
				fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) has conflicting logs with appended entires.\n", "[Append Entries-AppendEntriesHandler]", rf.state, rf.me, rf.currentTerm)
				break
			}
		}
	}
	// if not all entries are exactly identical to the follower's logs, else they are already appended.
	if conflict {
		rf.logs = rf.logs[:args.AEprevLogIndex+1]
		fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) occurs conflicting entries.\n", "[Append Entries-AppendEntriesHandler]", rf.state, rf.me, rf.currentTerm)
	}

	rf.logs = append(rf.logs[:args.AEprevLogIndex+1], args.AEentries...)
	reply.AERnextIndex = rf.getLastLogIndex() + 1
	reply.AERsuccess = true
	if args.AEleaderCommit > rf.commitIndex {
		if args.AEleaderCommit < rf.getLastLogIndex() {
			rf.commitIndex = args.AEleaderCommit
		} else {
			rf.commitIndex = rf.getLastLogIndex()
		}
		rf.chanCommit <- true
	}

	return
}

// send a heartbeat to specific peer.
func (rf *Raft) sendAppendEntries(p int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	DPrintf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) sends AppendEntries to server(%v).\n", "[Leader Election-sendAppendEntries]", rf.state, rf.me, rf.currentTerm, p)
	ok := rf.peers[p].Call("Raft.AppendEntriesHandler", args, reply)

	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.state != Leader {
		return ok
	}
	if !ok {
		return ok
	}
	if !reply.AERsuccess {
		if reply.AERterm > args.AEterm { // an stale leader, convert to follower.
			rf.state = Follower
			rf.currentTerm = reply.AERterm
			rf.votedFor = -1
			rf.votedCount = 0
			rf.persist()
			fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) converts to follower by AppendEntries RPC reply from server(%v, %v).\n", "[Append Entries-sendAppendEntries]", rf.state, rf.me, rf.currentTerm, p, reply.AERterm)
			return ok
		}
		if reply.AERterm == args.AEterm { // normal case.
			// if reply.AERnextIndex-1 < args.AEprevLogIndex {
			// update prevLogIndex, and resend heartbeat RPC
			// args.AEprevLogIndex = reply.AERnextIndex - 1
			// args.AEprevLogTerm = rf.logs[args.AEprevLogIndex].Term
			rf.nextIndex[p] = reply.AERnextIndex
			// fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) resends AppendEntries RPC after changing its prevLongIndex and prevLogTerm.\n", "[Append Entries-sendAppendEntries]", rf.state, rf.me, rf.currentTerm)
			// rf.sendAppendEntries(p, args, reply)
			return ok
			// }
		}
	} else {
		DPrintf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) appends Entries to server(%v, %v) successfully!\n", "[Append Entries-sendAppendEntries]", rf.state, rf.me, rf.currentTerm, p, reply.AERterm)
		if len(args.AEentries) != 0 {
			fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v)'s matchIndex for Server(%v, %v) advances from %v => %v.\n", "[#2B Append Entries-sendAppendEntries]", rf.state, rf.me, rf.currentTerm, p, reply.AERterm, rf.matchIndex[p], args.AEprevLogIndex+len(args.AEentries))
			rf.matchIndex[p] = args.AEprevLogIndex + len(args.AEentries)
		} else {
			rf.matchIndex[p] = args.AEprevLogIndex
		}
		if rf.nextIndex[p] < rf.matchIndex[p]+1 {
			DPrintf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v)'s nextIndex for Server(%v, %v) advances from %v => %v.\n", "[#2B Append Entries-sendAppendEntries]", rf.state, rf.me, rf.currentTerm, p, reply.AERterm, rf.nextIndex[p], rf.matchIndex[p]+1)
		}
		rf.nextIndex[p] = rf.matchIndex[p] + 1
	}
	return ok
}

// send a heartbeat(AppendEntries) RPC to all peers in parallel.
func (rf *Raft) broadcastAppendEntries() {
	// fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) begins broadcast AppendEntries RPCs to all peers.\n", "[Append Entries-broadcastAppendEntries]", rf.state, rf.me, rf.currentTerm)

	rf.mu.Lock()

	if rf.state != Leader {
		fmt.Printf(DEBUG_PREFIX_FORMAT+"Erorr to exit: Only leader can issue AppendEntries RPCs, its state is %v instead\n.", "[Append Entries-broadcastAppendEntries]", rf.state)
		return
	}

	lastLogIndex := rf.getLastLogIndex()
	baseLogIndex := rf.getBaseLogIndex()

	// update commitIndex.
	N := rf.commitIndex
	for p := rf.commitIndex + 1; p <= lastLogIndex; p++ {
		// fmt.Printf("=======peers number is %v============\n", len(rf.peers))
		// fmt.Printf("=======leader commitIndex is %v============\n", rf.commitIndex)
		count := 1
		for q := 0; q < len(rf.peers); q++ {
			if q != rf.me && rf.matchIndex[q] >= p && rf.logs[p].Term == rf.currentTerm {
				count++
			}
		}
		if count > len(rf.peers)/2 {
			N = p
		}
	}
	if rf.commitIndex < N {
		// logs[:N] has been commited, hence could be applied.
		fmt.Printf(DEBUG_PREFIX_FORMAT+"CommitIndex advances from %v => %v.\n", "[#2B Append Entries-broadcastAppendEntries]", rf.commitIndex, N)
		rf.commitIndex = N
		rf.chanCommit <- true
	}
	rf.mu.Unlock()

	for i := 0; i < len(rf.peers); i++ {
		if i != rf.me {
			rf.mu.Lock()
			prevLogTerm := -1
			prevLogIndex := rf.nextIndex[i] - 1
			var logEntries []LogEntry
			if baseLogIndex > BASE_LOG_INDEX-1 && prevLogIndex >= BASE_LOG_INDEX {
				// fmt.Printf("=======prevLogIndex=%v, len(rf.logs)=%v============\n", prevLogIndex, len(rf.logs))
				prevLogTerm = rf.logs[prevLogIndex].Term
			}
			if len(rf.logs) > BASE_LOG_INDEX {
				// copy unreplicated log entries to other peers.
				if lastLogIndex >= rf.nextIndex[i] {
					logEntries = append(logEntries, rf.logs[rf.nextIndex[i]:lastLogIndex+1]...)
				}
			}
			args := &AppendEntriesArgs{
				AEterm:         rf.currentTerm,
				AEleaderId:     rf.me,
				AEprevLogIndex: prevLogIndex,
				AEprevLogTerm:  prevLogTerm,
				AEleaderCommit: rf.commitIndex,
				AEentries:      logEntries,
			}
			rf.mu.Unlock()
			go func(j int, args_ *AppendEntriesArgs) {
				reply := &AppendEntriesReply{}
				rf.sendAppendEntries(j, args_, reply)
			}(i, args)
		}
	}
}

//
// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
//
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me
	rf.alive = true

	// Your initialization code here (2A, 2B, 2C).
	// 1. initialize raft server.
	rf.state = Follower
	rf.currentTerm = 0
	rf.votedFor = -1
	rf.votedCount = 0

	// Log entry index starts from BASE_LOG_INDEX
	for k := 0; k < BASE_LOG_INDEX; k++ {
		entry := LogEntry{
			LogIndex: k,
			Term:     0,
			Command:  -1,
		}
		rf.logs = append(rf.logs, entry)
	}

	rf.chanApply = applyCh
	rf.chanCommit = make(chan bool, chanSize)
	rf.chanHeartbeat = make(chan bool, chanSize)
	rf.chanRequestVote = make(chan bool, chanSize)
	rf.chanBeLeader = make(chan bool, chanSize)

	rf.commitIndex = BASE_LOG_INDEX - 1
	rf.lastApplied = BASE_LOG_INDEX - 1
	fmt.Printf(DEBUG_PREFIX_FORMAT+"Raft node %v created server(%v, %v, %v).\n", "[Make]", rf.me, rf.state, rf.me, rf.currentTerm)

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	go func() {
		for {
			rf.mu.Lock()
			if !rf.alive {
				fmt.Printf(DEBUG_PREFIX_FORMAT+"Have Server(%v, %v, %v) exit now...\n", "[Make]", rf.state, rf.me, rf.currentTerm)
				break
			}
			rf.mu.Unlock()
			switch rf.state {
			case Follower:
				{
					select {
					// auto reset timing if receive heartbeat RPCs without timeout.
					case <-rf.chanHeartbeat:
					// auto reset timing.
					case <-rf.chanRequestVote:
					// trun to #Candiddate immediately once timeout.
					case <-time.After(time.Duration(rand.Int63()%ElectionTimeOutRange+ElectionTimeOutBase) * time.Millisecond):
						fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) waits timeout, converts to Candidate.\n", "[Make]", rf.state, rf.me, rf.currentTerm)
						rf.state = Candidate
					}
				}
			case Candidate:
				{
					rf.mu.Lock()
					fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) restarts a new election.\n", "[Make]", rf.state, rf.me, rf.currentTerm)
					rf.currentTerm++
					rf.votedFor = rf.me
					rf.votedCount = 1
					rf.persist()
					rf.mu.Unlock()
					// start voting in parallel to try becoming a leader.
					go rf.broadcastRequestVote()
					select {
					// receive AppendEntries(hearbeat) RPC from a new leader, hence convert to follower.
					case <-rf.chanHeartbeat:
						rf.state = Follower
					// request vote RPC from other candidates.
					case <-rf.chanRequestVote:
					// i.e. votes could be split or network failure, no candidate steps forward in this term.
					case <-time.After(time.Duration(rand.Int63()%ElectionTimeOutRange+ElectionTimeOutBase) * time.Millisecond):
						fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) waits timeout.\n", "[Make]", rf.state, rf.me, rf.currentTerm)
					// receive majority of votes and hence become a leader. i.e. wins the election
					case <-rf.chanBeLeader:
						rf.mu.Lock()
						rf.state = Leader
						rf.votedFor = -1
						rf.votedCount = 0
						fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) begins reinitialize nextIndex and matchIndex.\n", "[Make]", rf.state, rf.me, rf.currentTerm)
						rf.nextIndex = make([]int, len(peers))
						rf.matchIndex = make([]int, len(peers))
						for i := 0; i < len(peers); i++ {
							rf.nextIndex[i] = rf.getLastLogIndex() + 1
							rf.matchIndex[i] = BASE_LOG_INDEX - 1
						}
						rf.mu.Unlock()
					}
				}
			case Leader:
				{
					go rf.broadcastAppendEntries()
					time.Sleep(HeartBeatInterval)
				}
			}
		}
	}()

	// a seperate goroutine for a node to apply logs once received chanCommitted message from leader.
	go func() {
		for {
			rf.mu.Lock()
			if !rf.alive {
				fmt.Printf(DEBUG_PREFIX_FORMAT+"Have Server(%v, %v, %v) exit now...\n", "[Make]", rf.state, rf.me, rf.currentTerm)
				break
			}
			rf.mu.Unlock()
			select {
			case <-rf.chanCommit:
				rf.mu.Lock()
				commitIndex := rf.commitIndex
				t := rf.lastApplied + 1
				for j := rf.lastApplied + 1; j <= commitIndex; j++ {
					applyMsg := ApplyMsg{
						CommandValid: true,
						Command:      rf.logs[j].Command,
						CommandIndex: rf.logs[j].LogIndex,
					}
					rf.chanApply <- applyMsg
					rf.lastApplied = j
				}
				fmt.Printf(DEBUG_PREFIX_FORMAT+"Server(%v, %v, %v) applied logs[%v, %v].\n", "[#2B Make]", rf.state, rf.me, rf.currentTerm, t, rf.commitIndex)
				rf.mu.Unlock()
			}
		}
	}()

	return rf
}
