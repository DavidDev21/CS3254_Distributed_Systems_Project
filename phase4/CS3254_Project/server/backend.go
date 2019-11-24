/*
	Name: David Zheng (dz1063)
	Course: CS3254
	Project Part 2 (Backend)
	backend.go will serve as the starting point for the backend server
*/
package main

import ("fmt"
		"flag"
		"net"
		"net/rpc"
		"os"
		"errors"
		"sync"
		"strings"
		CO "CS3254_Project/custom_objects")

// Performs RPC Dial to address under the indicated network protocol
// Exits the program if there was an error
func DialWithCheck(protcol, address string) (*rpc.Client, error) {
	conn, err := rpc.Dial(protcol, address)

	if (err != nil) {
		fmt.Println(err.Error())
		return conn, errors.New("Connection To Replica Failed")
	}

	return conn, nil
}

/*
	Client with leader interaction
		The client should interact with leader through a specific RPC call
		that takes in what command they would like to execute along with the arguments and response structs like normal
		
		the leader should simply (after it proccess all the RAFT protocol normal operations and is ready to apply and commit the operation)
		pass the arguments along to the already existing RPC functions from phase 2
		Which should effectively pass back the results accordingly
*/

/*
	Crucial thoughts

	1.) Leader needs to keep track of all the log indices for each follower which is the purpose of nextIndex[]
			This allows the leader to be aware of what the followers need in their logs
			and also allows leader to know when it is safe to accept client requests again. (when we have a quorum of nodes that is fully synced up)
			// basically the nextIndex log matches up with what we have as the leader
	2.) AppendEntries should be sending one entry at a time based on whatever the follower is missing. as indicated by the nextIndex[i], i being the follower
			For each failed AppendEntries call to the follower, we decrement the nextIndex[i] which is a indicator the follower is missing entries or doesn't match
			We keep decrement until we get a successful AppendEntries indicating that we reached a point in the follower's log where it matches what we have

			The replication strategy from here is to increment on successful of each AppendEntries call and basically incrementally build up the 
			log of the follower
			(in the worst case, we rebuild the whole log with 2n messages, (n being the length of the log))

	3.) Client Leader interaction
			have an RPC method the client will be able to use to talk to the backend
			-- The RPC method should take in what command client wants and the arguments
			-- It should check what state we are in, If we are not Leader, then reply back with the Leaders ID (nodeID)
			-- If we are the leader, then we send off the given arguments from client to a separate thread (the RAFT thread) for processing
					-- Results should be handled or passed through a one capacity channel
					-- This way the RPC method will know when to send back the results to the client
		RAFT must run on a separate thread because of:
			1.) the built in heartbeat mechanic of RAFT, which needs to keep going regardless of client interaction
			2.) All backend replicas must run its own logic independent of the client
		This also forces the client to wait for us to finish, and also allows us to process multiple clients requests in a relatively safe matter
		-- The results channel can be limited to size 1 so we can process one client at a time, while acknowledging multiple clients at once
		-- Effectively the concurrent client requests will be processed based on the non-deterministic behavior

	4.) Timeout mechancism
			Ideally, have a separate thread only for timeout
			Where there is a timeout, we look at our current state, and then take steps to transition to a different state
			Ex:
				at the moment of the timeout, 
				a.) if we are still the candidate, rerun the candidate protocol
				b.) if we are in the follower state, then change state to candidate, and run candidate protocol
				c.) if we are in the leader state, we shouldn't be timing out as a leader

			Check playground for a possible skeleton that works
*/


// Raft Stuff
const FOLLOWER_STATE = "follower"
const LEADER_STATE = "leader"
const CANDIDATE_STATE = "candidate"

// We need the arguments that come with the commands
// In order to replicate the state of the database
type CommandArgs struct {

}

type LogEntry struct {
	termNumber int
	command string
	args CommandArgs
}

type Raft struct {
	ID string // to make it easier to ID ourselves
	currentTerm int
	votedFor string // Who did we vote for during election
	committedIndex int // the highest Log Entry that was known to be committed, i.e, to have been executed by leader
	lastApplied int // the highest Log Entry that was actually executed on our local FSM (the database)
	log []LogEntry // replica log for FSM, holds the termNumber and command from leader

	// In the event of a crash, (log not in disk)
	// Might just be easier to grab the whole log from the current leader
	nextIndex []int // track of what the next log entry should be sent to i'th follower
	// matchIndex would be for replication efficiency in the event the follower is significantly behind the leader
	matchIndex []int // track of the last index where each follower matches with the leader's log
	state string

	quorumSize int // the minimum quorum we need given our initial configuration

	replicas []string
}

/* RPC HANDLERS */
type AppendRequest struct {

}
type RaftResponse struct {

}

func (self *Raft) AppendEntries() {

}

// Leader Election Methods
type RequestVoteArgs struct {
	term int
	candidateID string
	lastLogIndex int
	lastLogTerm int
}

type RequestVoteResponse struct {
	term int
	voteGranted bool
}

// Candidate method calls VIA RPC
// Get Vote from follower
/*
	Input Params:
		term = the candidate's current term
		candidateID = the ID of whoever is the candidate requesting ( might be ID or the candidate IP addr, idk)
		lastLogIndex = the index of the last log entry on the candidate's log
		lastLogTerm = the term of the last log entry on the candidate's log
	Response:
		followerTerm (int) = the follower's current term
		voteGranted (bool) = whether the follower granted the vote to the requesting candidate
*/

// Work In Progress: current skelelton
// self: here should be the receiver end meaning a follower
func (self *Raft) RequestVote(term int, candidateID string, lastLogIndex int, lastLogTerm int) (int, bool) {
	// Case: we already voted for the candidate that is requesting (in the event of network failure)
	if (self.votedFor == candidateID) {
		return self.currentTerm, true
	}

	// Case: we already voted for someone else
	if (self.votedFor != "") {
		return self.currentTerm, false
	}

	// First Check if Caller's term is not behind ours
	// If so, we don't give them our vote since they are not eligible for leadership
	if (term < self.currentTerm) {
		return self.currentTerm, false
	}

	// We grant the vote if we haven't voted for anyone, or if we already voted for candidateID.
	// The candidate's log should be at least up to date in respects to our log
	/*
			Snippet from the RAFT paper

			If the logs have last entries with different terms, then
			the log with the later term is more up-to-date. If the logs
			end with the same term, then whichever log is longer is
			more up-to-date.
	*/
	//		- lastLogTerm >= self.lastLogTerm (if our last log entry term is higher, that means the candidate's log doesn't match with our expectations)
	//				Higher term indicates a more true or updated log since high terms require a quorum of followers to elect a leader
	//				Lower terms means the candidate potentially lost connection to the majority of the cluster and was never informed of the new term
	// 		- lastLogIndex >= self.LogIndex (candidate's log should at least be in the same length as ours)

	currentLastIndex := len(self.log)-1

	if (lastLogTerm > self.log[currentLastIndex].termNumber) {
		return self.currentTerm, true
	}

	if (lastLogTerm == self.log[currentLastIndex].termNumber && lastLogIndex > currentLastIndex) {
		return self.currentTerm, true
	}

	return self.currentTerm, false
}

// Ideally: calls another RPC message on the receiver (follower)
// to get a vote from them
// GetVotes() should return the number of votes we got back from the other replicas
// We count those replicas that we can't reach as a no vote
// Self: here should be the candidate in this case, ourselves
func (self *Raft) GetVotes() int {

	// vote for self
	self.votedFor = self.ID
	numVotes := 1

	var failedConnections = make(map[string]bool)

	// For each replica, attempt to get a vote
	for _, replicaAddr := range self.replicas {
		// Connect to replica
		conn, err := DialWithCheck("tcp", replicaAddr)
		defer conn.Close()

		// If we failed, note it so we can try again later
		if (err != nil) {
			// adding replicaAddr to map
			failedConnections[replicaAddr] = true
		} else {
			req, res := new(Request), new(Response)

			// RPC Call
			conn.Call("Raft.RequestVote", req, res)
		}
	}

	// We win if we got enough votes on the first try (assume we were able to contact a quorum)
	if (numVotes >= self.quorumSize) {
		self.state = LEADER_STATE
		return 0 // Exit
	}

	// If we are not able to contact a quorum initially, then we need to try to get the votes from previously failed attempts
	// And keep trying until someone either informs us of a new leadership, or we timeout and try again
	// While we still think we are the candidate, keep trying to get the votes that you need
	for (self.state == CANDIDATE_STATE) {
		// We win
		if (numVotes >= self.quorumSize) {
			self.state = LEADER_STATE
			return 0 // Exit
		}

		for replicaAddr, _ := range failedConnections {
			// Connect to replica
			conn, err := DialWithCheck("tcp", replicaAddr)
			defer conn.Close()

			// If we can connect then we try to do the RPC call
			if (err == nil) {
				req, res := new(Request), new(Response)

				// RPC Call
				conn.Call("Raft.RequestVote", req, res)
			}
		}
	}

	return 0
}

// Raft Infrastructure Code
// Setsup initial state of raft node
func (self *Raft) RaftSetup(nodeID string, numOfReplicas int) {
	self.state = FOLLOWER_STATE
	self.ID = nodeID
	self.currentTerm = 1
	self.votedFor = ""
	self.committedIndex = 0
	self.lastApplied = 0
	self.quorumSize = (numOfReplicas/2) + 1
}

// Ideally running this in a separate thread (or basically the main thread)
// Separate from the RPCs
// Self: is the current Raft instance on this machine
func (self *Raft) RunRaft(nodeID string, numReplicas int) {

	self.RaftSetup(nodeID, numReplicas)

	for {
		switch self.state {
		case FOLLOWER_STATE :
			fmt.Println("I am follower")
		case LEADER_STATE :
			fmt.Println("I am leader")
		case CANDIDATE_STATE :
			fmt.Println("I am in candidate")
		}

	}

}

// Jumpstarts the Election process for us
func StartElection () {

}

// Algo for each state
func RunFollower() {

}

func RunLeader() {

}

func RunCandidate() {

}

// End of Raft

// Constants
const NUM_THREADS = 5
const DEFAULT_BACKEND_ADDR = "localhost:8090"
const DEFAULT_PORT = 8090
const HEARTBEAT_PORT = ":8091"

// Global Var
var database []CO.Product

// WaitGroup
var threadGroup sync.WaitGroup

// Mutex
var Mutex = &sync.Mutex{}

// Simply accept a connection from whoever wants to know if the server is alive
func heartBeat(protocol string) {
	sock, err := net.Listen(protocol, HEARTBEAT_PORT)
	if (err != nil) {
		fmt.Println(err)
	}
	for {
		conn, err := sock.Accept()
		fmt.Println("Sending heartbeat")
		if (err != nil) {
			fmt.Println(err)
		}
		defer conn.Close()
	}
}


// Removes an item from the database based on the product ID
// the ... acts like a spread operator in javascript
// append takes a variable amount of arguments and ... allows use to the db slice as input
// if no ID found, nothing happens
func removeItem(db []CO.Product, id int) []CO.Product {
	for i, record := range(db) {
		if record.ID == id {
			db = append(db[:i], db[i+1:]...)
		} 
	}

	return db
}

// Does binary search to find the item in our DB list
// Items on the list are always in ascending order
func getItem(db *[]CO.Product, target int) (*CO.Product, error) {
	left, right := 0, len(*db)-1

	for left <= right {
		mid := (left+right)/2
		// return the address to the product object in the database
		if ((*db)[mid].ID == target) {
			return &(*db)[mid], nil
		} else if ((*db)[mid].ID < target) {
			left = mid+1
		} else {
			right = mid-1
		}
	}
	return &CO.Product{}, errors.New("ID not found")
}

/* RPC HANDLERS */
type DB struct {}
type Request struct {
	ProductID int
	ProductInfo CO.Product
}
type Response struct {
	ProductList []CO.Product
	ProductItem CO.Product
}

// Returns the whole database to the client
func (t *DB) GetProductList(req Request, res *Response) error {
	Mutex.Lock()
	res.ProductList = database
	Mutex.Unlock()
	fmt.Println("returning a database to frontend")
	return nil
}

// Returns a Product to the client based on given Product ID
func (t *DB) GetProduct(req Request, res *Response) error {
	Mutex.Lock()
	item, err := getItem(&database, req.ProductID)

	if (err != nil) {
		return err
	}

	res.ProductItem = *item
	Mutex.Unlock()

	fmt.Println("return a product to frontend")
	return nil
}

// Edit Product Info based on given Product ID
func (t *DB) EditProduct(req Request, res *Response) error {
	Mutex.Lock()
	item, err := getItem(&database, req.ProductID)

	if (err != nil) {
		return err
	}

	item.Name = req.ProductInfo.Name
	item.Seller = req.ProductInfo.Seller
	item.Condition = req.ProductInfo.Condition
	item.Description = req.ProductInfo.Description
	item.Price = req.ProductInfo.Price

	Mutex.Unlock()

	fmt.Println("Editing Product ID: ", req.ProductID)
	return nil
}

// Creates a new product and append to database based on info given
func (t *DB) AddProduct(req Request, res *Response) error {
	price := req.ProductInfo.Price
	name := req.ProductInfo.Name
	description := req.ProductInfo.Description
	condition := req.ProductInfo.Condition
	seller := req.ProductInfo.Seller

	Mutex.Lock()
	database = append(database, CO.NewProduct(price, name, description, condition, seller))
	Mutex.Unlock()
	return nil
}

// Delete a product from database based on ProductID
func (t *DB) DeleteProduct(req Request, res *Response) error {

	Mutex.Lock()
	// Check if ID is valid
	_, err := getItem(&database, req.ProductID)

	if (err != nil) {
		return err
	}

	database = removeItem(database, req.ProductID)
	Mutex.Unlock()
	return nil
}

// Backend Flag
type arrayFlag []string

func (i *arrayFlag) String() string {
	return ""
}

func (i *arrayFlag) Set(value string) error {
	*i = append(*i, strings.Split(value, ",")...)
	return nil
}

// Validates and transforms ip address if needed
func transformAddr (addr string) (string, bool) {
	backendAddrSplit := strings.Split(addr, ":")

	// With IP but no port indicated.
	if (len(backendAddrSplit) == 2 && len(backendAddrSplit[1]) == 0) {
		return "", false
	}

	// Address Validation to backend
	// Just assume the format is wrong if there are more than one colon
	if (strings.LastIndex(addr, ":") == -1 || len(backendAddrSplit) > 2) {
		return "", false
	} else if (strings.LastIndex(addr, ":") == 0) { // This is if user only enters port num
		if (len(addr) == 1) { // "only : case"
			addr = DEFAULT_BACKEND_ADDR
		} else {
			addr = "localhost" + addr
		}
	}

	return addr, true
}

func main() {
	// Add some initial items into our db
	database = append(database, CO.NewProduct(999, "MacBook Air 2019", "This is the new Apple MacBook Air 2019, with Great Specs!", "New","Apple"))
	database = append(database, CO.NewProduct(300, "Nintendo Switch", "This is the most awesome switch in the world", "Good", "Nintendo (the guy next door)"))

	// Register a handler
	rpc.Register(new(DB))

	// Start up thread to listen on heartbeat port
	go heartBeat("tcp")

	// Defines the listen flag
	var portNum int
	var backendReplicas arrayFlag
	var nodeID string 

	flag.IntVar(&portNum, "listen", 8090, "This is the port number the application will listen on")
	flag.Var(&backendReplicas, "backend", "This is a list of all the other replicas on the backend")
	flag.StringVar(&nodeID, "id", "", "This is the public IP address for this node")

	// Parases the command line for flags
	flag.Parse()
	fmt.Println(portNum)

	fmt.Println(backendReplicas)
	// Checks all the IP addresses to the backend replicas
	for i := 0; i < len(backendReplicas); i++ {
		fmt.Println(backendReplicas[i])
		formattedAddr, valid := transformAddr(backendReplicas[i])

		if (valid == false) {
			fmt.Println("backend_flag: Wrong address format")
			fmt.Println("backend_flag: Valid address format: <ip>:<port>")
			fmt.Println("backend_flag: ", backendReplicas[i])
			os.Exit(1)
		}

		backendReplicas[i] = formattedAddr
	}

	fmt.Println(backendReplicas)

	servicePort := fmt.Sprintf(":%d", portNum)

	// Listen on the servicePort
	listener, err := net.Listen("tcp", servicePort)

	if (err != nil){
		fmt.Println(err.Error())
		os.Exit(1)
	}

	fmt.Println("Server now listening on", servicePort)
	// Create multi threads to listen to requests
	threadGroup.Add(NUM_THREADS)

	for i := 0; i < NUM_THREADS; i++ {
		go rpc.Accept(listener)
	}

	// Have main thread wait forever until we force backend to quit
	threadGroup.Wait()
}