/*
	Name: David Zheng (dz1063)
	Course: CS3254
	Project Part 1
	main.go will serve as the starting point for the web app
*/

package main

import ("fmt"
		"github.com/kataras/iris"
		"flag"
		"os"
		"net"
		"net/rpc"
		"strings"
		"time"
		"errors"
		CO "CS3254_Project/custom_objects"
		)

// Constants
const DEFAULT_BACKEND_ADDR = "localhost:8090"
const DEFAULT_PORT = 8080
const DEFAULT_HEARTBEAT_PORT = "8091"

// Flags
var portNum int
var backendReplicas arrayFlag

// Global Vars
var leader string // to know who the leader is on the backend

// timeout (used to give up trying to connect to backend)
const TIMEOUT_PERIOD = 5000 // in milliseconds
var timer <-chan time.Time


/* RPC interfaces */
type DB struct {}
type Request struct {
	ProductID int
	ProductInfo CO.Product
}
type Response struct {
	ProductList []CO.Product
	ProductItem CO.Product
}

// Used to communicate to the raft cluster
type CommandArgs struct {
	ProductID int
	ProductInfo CO.Product
}

type CommandResponse struct {
	ProductList []CO.Product
	ProductItem CO.Product
}

type SendCommandArgs struct {
	Command string // what command to execute
	Args CommandArgs // the arguments for the command
}

type SendCommandResponse struct {
	LeaderID string // to tell the client who the leader is
	Success bool // whether the command was successful
	Response CommandResponse // the result of the command being executed
}

func SendRaftRequest(req *SendCommandArgs, res *SendCommandResponse) bool {
	gotResult := false

	for gotResult == false {
		select {
		case <- timer:
			fmt.Println("Request Timed out")
			return false
		default:
			// Try to connect with the leader first
			conn, err := DialWithCheck("tcp", leader)

			// Failed to connect to leader. means the leader potentially changed
			if (err != nil) {
				fmt.Println("Failed connection to leader")
				for _, replicaAddr := range backendReplicas {
					conn, err := DialWithCheck("tcp", replicaAddr)

					// If we connected to a backend replica, send our request over to get the updated leader
					if (err == nil) {
						rpcError := conn.Call("Raft.SendCommand", req, res)
						conn.Close()
						if (rpcError == nil) {
							leader = res.LeaderID
							fmt.Println("New leader found")
							break
						}
					}
				}
			} else {
				// Send request to leader
				rpcError := conn.Call("Raft.SendCommand", req, res)
				conn.Close()
				if (rpcError != nil) {
					fmt.Println("SendRaftRequest: Error sending request to leader")
					fmt.Println(rpcError)
					return false
				}

				// The leadership changed, we basically keep asking the "old leader"
				// which could very well be a follower or a candidate
				// If we lose connection to the node, we connect to a working node and ask it who the new leader is
				if (res.Success == false) {
					leader = res.LeaderID
					fmt.Println("New Leader Found from old leader")
				} else {
					// We got the result back from the leader
					gotResult = true
				}
			}
		}
	}

	return true
}

// Performs RPC Dial to address under the indicated network protocol
// Exits the program if there was an error
func DialWithCheck(protcol, address string) (*rpc.Client, error) {
	conn, err := rpc.Dial(protcol, address)

	if (err != nil) {
		fmt.Println(err.Error())
		return conn, errors.New("Connection To Backend Failed")
		//os.Exit(1)
	}

	return conn, nil
}

// Worker to check the health of the backend (heartbeat)
func CheckBackendHealth(protcol, address string) {
	for {
		_, err := net.Dial(protcol, address + ":" + DEFAULT_HEARTBEAT_PORT)
		
		if (err != nil) {
			fmt.Println("Detected Failure on", address, "at", time.Now().UTC())
		}

		// A minute before checking on the health of the backend
		time.Sleep(60 * time.Second)
	}
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
	flag.IntVar(&portNum, "listen", DEFAULT_PORT, "this is the port number the application will listen on")
	flag.Var(&backendReplicas, "backend", "This is the list of all the addresses to the backend replicas. <ip>:<port>")
	
	// Parses the command line for flags
	flag.Parse()
	fmt.Println("Frontend Port:", portNum)

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

	leader = backendReplicas[0] // just pick a random one to try out first

	// // Just in case the port is the heartbeat port
	// if (backendAddrSplit[1] == DEFAULT_HEARTBEAT_PORT) {
	// 	fmt.Println("backend_flag: Port set to default heartbeat port, please select a different port")
	// 	os.Exit(1)
	// }
	// // Creates separate thread to get heartbeat pings from backend
	// // Note: backendAddr would consist of addr and port after validation check
	// go CheckBackendHealth("tcp", strings.Split(backendAddr, ":")[0])

	// fmt.Println("Backend Addr:" , backendAddr)

	// IRIS setup
	app := iris.Default()
	serverAddr := fmt.Sprintf(":%d", portNum)

	// View template registeration
	app.RegisterView(iris.HTML("../views", ".html"))

	// Tells IRIS where is the styles folder
	app.HandleDir("/styles", "../views/styles")

	// GET method for route "/"
	app.Handle("GET", "/", func(ctx iris.Context) {
		req, res := new(SendCommandArgs), new(SendCommandResponse)

		req.Command = "GetProductList"
		timer = time.After(time.Duration(TIMEOUT_PERIOD) * time.Millisecond)
		success := SendRaftRequest(req, res)

		if (success == false) {
			fmt.Println("GetProductList: Failed request")
			ctx.Redirect("/", 400)
		} else {
			// Render page
			ctx.ViewData("database", res.Response.ProductList)
			ctx.View("index.html")
		}
	})

	// GET edit product page
	app.Handle("GET", "/edit/{ID: int}", func(ctx iris.Context) {
		// Get ID Parameter
		id, err := ctx.Params().GetInt("ID")

		if (err != nil) {
			fmt.Println("GetProduct: Failed request")
			ctx.Redirect("/", 400)
		} else {
			req, res := new(SendCommandArgs), new(SendCommandResponse)

			req.Command = "GetProduct"
			req.Args.ProductID = id

			timer = time.After(time.Duration(TIMEOUT_PERIOD) * time.Millisecond)
			success := SendRaftRequest(req, res)
			
			if (success == false) {
				fmt.Println("GetProduct: Failed Request")
				ctx.Redirect("/", 400)
			} else {
				// Render page
				ctx.ViewData("product", res.Response.ProductItem)
				ctx.View("editProduct.html")
			}
		}
	})
	
	// POST edit product page
	// Handles the update of a product
	app.Handle("POST", "/edit/{ID: int}", func(ctx iris.Context) {
		// Get ID Parameters
		id, err := ctx.Params().GetInt("ID")

		if (err != nil) {
			fmt.Println("EditProduct: Failed request")
			ctx.Redirect("/", 400)
		} else {
			req, res := new(SendCommandArgs), new(SendCommandResponse)

			req.Command = "EditProduct"

			// Args for RPC
			req.Args.ProductID = id
			req.Args.ProductInfo.Name = ctx.PostValue("name")
			req.Args.ProductInfo.Seller = ctx.PostValue("seller")
			req.Args.ProductInfo.Condition = ctx.PostValue("condition")
			req.Args.ProductInfo.Description = ctx.PostValue("description")
			req.Args.ProductInfo.Price, _ = ctx.PostValueInt("price")

			timer = time.After(time.Duration(TIMEOUT_PERIOD) * time.Millisecond)
			success := SendRaftRequest(req, res)

			if (success == false) {
				fmt.Println("EditProduct: Failed Request")
				ctx.Redirect("/", 400)
			} else {
				// return home
				ctx.Redirect("/", 302)
			}
		}
	})


	// GET Add product
	app.Handle("GET", "/createProduct", func(ctx iris.Context) {
		ctx.View("createProduct.html")
	})
 
	// POST Add product
	app.Handle("POST", "/createProduct", func(ctx iris.Context) {
		req, res := new(SendCommandArgs), new(SendCommandResponse)

		req.Command = "AddProduct"

		// Args for RPC
		req.Args.ProductInfo.Name = ctx.PostValue("name")
		req.Args.ProductInfo.Seller = ctx.PostValue("seller")
		req.Args.ProductInfo.Condition = ctx.PostValue("condition")
		req.Args.ProductInfo.Description = ctx.PostValue("description")
		req.Args.ProductInfo.Price, _ = ctx.PostValueInt("price")

		timer = time.After(time.Duration(TIMEOUT_PERIOD) * time.Millisecond)
		success := SendRaftRequest(req, res)
	
		if (success == false) {
			fmt.Println("AddProduct: Request Failed")
		}
	
		// return home
		ctx.Redirect("/", 302)

	})

	// POST Delete product
	app.Handle("POST", "/delete/{ID: int}", func(ctx iris.Context) {
		// Get ID Parameter
		id, err := ctx.Params().GetInt("ID")

		if (err != nil) {
			ctx.Redirect("/", 400)
		} else {
			req, res := new(SendCommandArgs), new(SendCommandResponse)

			req.Command = "DeleteProduct"

			// Args for RPC
			req.Args.ProductID = id

			timer = time.After(time.Duration(TIMEOUT_PERIOD) * time.Millisecond)
			success := SendRaftRequest(req, res)

			if (success == false) {
				fmt.Println("DeleteProduct: Failed Request")
				ctx.Redirect("/", 400)
			} else {
				// return home
				ctx.Redirect("/", 302)
			}
		}
	})

	// IRIS listening on port
	app.Run(iris.Addr(serverAddr), iris.WithoutServerError(iris.ErrServerClosed))
}
