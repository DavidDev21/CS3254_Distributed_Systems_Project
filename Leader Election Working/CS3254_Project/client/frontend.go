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
	// Defines the listen, backend flag
	var portNum int
	var backendReplicas arrayFlag

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

	/*
	// GET method for route "/"
	app.Handle("GET", "/", func(ctx iris.Context) {
		// Connect to backend
		conn, err := DialWithCheck("tcp", backendAddr)
		defer conn.Close()

		if (err != nil) {
			fmt.Println(err)
			ctx.Redirect("/", 400)
		} else {
			req, res := new(Request), new(Response)

			// RPC Call
			conn.Call("DB.GetProductList", req, res)
	
			// Render page
			ctx.ViewData("database", res.ProductList)
			ctx.View("index.html")
		}
	})

	// GET edit product page
	app.Handle("GET", "/edit/{ID: int}", func(ctx iris.Context) {
		// Get ID Parameter
		id, err := ctx.Params().GetInt("ID")

		if (err != nil) {
			ctx.Redirect("/", 400)
		} else {
			// Connect to backend
			conn, err := DialWithCheck("tcp", backendAddr)
			defer conn.Close()

			if (err != nil) {
				fmt.Println(err)
				ctx.Redirect("/", 400)
			} else {

				req, res := &Request{ProductID: id}, new(Response)

				// RPC Call
				err = conn.Call("DB.GetProduct", req, res)
	
				if (err != nil) {
					fmt.Println(err)
				}
	
				// Render page
				ctx.ViewData("product", res.ProductItem)
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
			ctx.Redirect("/", 400)
		} else {
			// Connect to backend
			conn, err := DialWithCheck("tcp", backendAddr)
			
			defer conn.Close()

			if (err != nil) {
				fmt.Println(err)
				ctx.Redirect("/", 400)
			} else {

				req, res := new(Request), new(Response)

				// Args for RPC
				req.ProductID = id
				req.ProductInfo.Name = ctx.PostValue("name")
				req.ProductInfo.Seller = ctx.PostValue("seller")
				req.ProductInfo.Condition = ctx.PostValue("condition")
				req.ProductInfo.Description = ctx.PostValue("description")
				req.ProductInfo.Price, _ = ctx.PostValueInt("price")
	
				// RPC call
				err = conn.Call("DB.EditProduct", req, res)
	
				if (err != nil) {
					fmt.Println(err)
				}
				
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
		// Connect to backend
		conn, err := DialWithCheck("tcp", backendAddr)
		defer conn.Close()

		if (err != nil) {
			fmt.Println(err)
			ctx.Redirect("/", 400)
		} else {

			req, res := new(Request), new(Response)

			// Args for RPC
			req.ProductInfo.Name = ctx.PostValue("name")
			req.ProductInfo.Seller = ctx.PostValue("seller")
			req.ProductInfo.Condition = ctx.PostValue("condition")
			req.ProductInfo.Description = ctx.PostValue("description")
			req.ProductInfo.Price, _ = ctx.PostValueInt("price")
	
			// RPC Call
			err = conn.Call("DB.AddProduct", req, res)
	
			if (err != nil) {
				fmt.Println(err)
			}
	
			// return home
			ctx.Redirect("/", 302)
		}
	})

	// POST Delete product
	app.Handle("POST", "/delete/{ID: int}", func(ctx iris.Context) {
		// Get ID Parameter
		id, err := ctx.Params().GetInt("ID")

		if (err != nil) {
			ctx.Redirect("/", 400)
		} else {
			// Connect to backend
			conn, err := DialWithCheck("tcp", backendAddr)

			defer conn.Close()

			if (err != nil) {
				fmt.Println(err)
				ctx.Redirect("/", 400)
			} else {

				req, res := new(Request), new(Response)

				// Args for RPC
				req.ProductID = id
	
				// RPC Call
				err = conn.Call("DB.DeleteProduct", req, res)
	
				if (err != nil) {
					fmt.Println(err)
				}
	
				// return home
				ctx.Redirect("/", 302)
			}
		}
	})

	*/

	// IRIS listening on port
	app.Run(iris.Addr(serverAddr), iris.WithoutServerError(iris.ErrServerClosed))
}
