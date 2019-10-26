package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/Songmu/prompter"
)

//Config defines model for storing account details in database
type Config struct {
	Name    string             `json:"name"`
	Version string             `json:"version"`
	License string             `json:"license"`
	Client  Client             `json:"client"`
	Channel map[string]Channel `json:"channels"`
}

type Client struct {
	TlsEnable bool   `json:"tlsEnable"`
	AdminUser string `json:"adminUser"`
}

type Channel struct {
	Peers map[string]Peer `json:"peers"`
}

type Peer struct {
}

func main() {
	config := Config{}

	config.Name = "first-network"
	config.Version = "1.0.0"
	config.License = "Apache-2.0"
	config.Client.AdminUser = "admin"

	if prompter.YN("TLS enabled ?", true) {
		config.Client.TlsEnable = true
	} else {
		config.Client.TlsEnable = false
	}

	cid := prompter.Prompt("Enter channel ID", "mychannel")

	config.Channel = make(map[string]Channel)
	config.Channel[cid] = Channel{Peers: make(map[string]Peer)}
	channel := config.Channel[cid]
	channel.Peers["peer0.org1.example.com"] = Peer{}

	fabricLoc := prompter.Choose("Where is your Fabric network located?", []string{"local", "remote"}, "local")

	explorerBoot := prompter.Choose("How to bring up explorer?", []string{"source", "docker"}, "source")

	fmt.Println(fabricLoc, ":", explorerBoot)

	bytes, err := json.Marshal(config)
	if err != nil {
		return
	}
	fmt.Println(string(bytes))
	err = ioutil.WriteFile(config.Name+".json", []byte(string(bytes)), 0644)
	if err != nil {
		return
	}

	// //Parse json request body and use it to set fields on config
	// //Note that config is passed as a pointer variable so that it's fields can be modified
	// err := json.NewDecoder(r.Body).Decode(&config)
	// if err != nil{
	// 	panic(err)
	// }

	// //Set CreatedAt field on user to current local time
	// user.CreatedAt = time.Now().Local()

	// //Marshal or convert user object back to json and write to response
	// userJson, err := json.Marshal(user)
	// if err != nil{
	// 	panic(err)
	// }

	// //Set Content-Type header so that clients will know how to read response
	// w.Header().Set("Content-Type","application/json")
	// w.WriteHeader(http.StatusOK)
	// //Write json response back to response
	// w.Write(userJson)

}
