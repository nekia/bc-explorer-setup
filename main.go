package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/Songmu/prompter"
	"github.com/dixonwille/wmenu"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/kr/pretty"
	"golang.org/x/net/context"
	"gopkg.in/yaml.v2"
)

//Config defines model for storing account details in database
type Config struct {
	Name    string             `json:"name"`
	Version string             `json:"version"`
	License string             `json:"license"`
	Client  Client             `json:"client"`
	Channel map[string]Channel `json:"channels"`
}

type Crypto struct {
	Name          string
	Domain        string
	EnableNodeOUs bool
	Specs         []Spec
}

type Spec struct {
	Hostname   string
	CommonName string
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

func pullCli(ver string) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	out, err := cli.ImagePull(ctx, "hyperledger/fabric-tools:"+ver, types.ImagePullOptions{})
	if err != nil {
		panic(err)
	}

	defer out.Close()

	io.Copy(os.Stdout, out)
}

func listCli(peer string) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	var peerContainer types.Container
	c := types.ContainerListOptions{}
	if list, err := cli.ContainerList(ctx, c); err != nil {
		panic(err)
	} else {
		for _, container := range list {
			if strings.Split(container.Names[0], "/")[1] == peer {
				fmt.Printf("Found : %s\n", peer)
				peerContainer = container
				break
			}
		}
	}

	cc := types.ExecConfig{AttachStdout: true, AttachStderr: true, Cmd: []string{"peer", "channel", "list"}}
	execID, _ := cli.ContainerExecCreate(ctx, peerContainer.ID, cc)

	config := types.ExecStartCheck{}
	res, err := cli.ContainerExecAttach(ctx, execID.ID, config)
	if err != nil {
		panic(err)
	}

	err = cli.ContainerExecStart(ctx, execID.ID, types.ExecStartCheck{})
	if err != nil {
		panic(err)
	}
	content, _, _ := res.Reader.ReadLine()
	fmt.Println(string(content))
}

func main() {
	config := Config{}

	config.Name = "first-network-generated"
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

	org := make(map[interface{}]interface{})

	actOrgFunc := func(opts []wmenu.Opt) error {
		for _, opt := range opts {
			org = opt.Value.(map[interface{}]interface{})
			fmt.Printf("%s has an id of %d. %s\n", opt.Text, opt.ID, org["MSPDir"].(string))
		}
		return nil
	}
	menu := wmenu.NewMenu("Choose an organization used to connect network")
	menu.Action(actOrgFunc)
	// menu.AllowMultiple()
	// menu.SetSeparator(",")

	configdata, _ := ioutil.ReadFile("./configtx.yaml")
	m := make(map[interface{}]interface{})
	yaml.Unmarshal(configdata, &m)
	configurationsarray := m["Organizations"].([]interface{})
	for _, e := range configurationsarray {
		ee := e.(map[interface{}]interface{})
		menu.Option(ee["Name"].(string), ee, false, nil)
		// pretty.Printf("--- configurations:\n%# v\n\n", ee)
	}

	err := menu.Run()
	if err != nil {
		log.Fatal(err)
	}

	mspDirPath := strings.Split(org["MSPDir"].(string), "/")
	domain := mspDirPath[len(mspDirPath)-2]
	fmt.Println(domain)

	orgCrypto := make(map[interface{}]interface{})

	cryptoConfig, _ := ioutil.ReadFile("./crypto-config.yaml")
	n := make(map[interface{}]interface{})
	yaml.Unmarshal(cryptoConfig, &n)
	peerOrgArray := n["PeerOrgs"].([]interface{})
	for _, e := range peerOrgArray {
		ee := e.(map[interface{}]interface{})
		pretty.Printf("--- Crypto Config:\n%# v\n\n", ee)
		if ee["Domain"].(string) == domain {
			orgCrypto = ee
			break
		}
	}

	fmt.Println(orgCrypto["Name"].(string))

	if v, ok := orgCrypto["Specs"]; ok {
		array := v.([]interface{})
		for _, e := range array {
			spec := e.(map[interface{}]interface{})
			fmt.Println("HOSTNAME:" + spec["Hostname"].(string))
			if cmnName, ok := spec["CommonName"]; ok {
				fmt.Println("CN:" + cmnName.(string))
			} else {
				fmt.Println("CN:" + spec["Hostname"].(string) + "." + orgCrypto["Domain"].(string))
			}
		}
	}

	var dfltPeer string
	actPeerFunc := func(opts []wmenu.Opt) error {
		for _, opt := range opts {
			dfltPeer = opt.Value.(string)
			fmt.Printf("%s is selected\n", dfltPeer)
		}
		return nil
	}
	menu = wmenu.NewMenu("Choose a peer to use as default peer")
	menu.Action(actPeerFunc)
	// menu.AllowMultiple()
	// menu.SetSeparator(",")

	if v, ok := orgCrypto["Template"]; ok {
		template := v.(map[interface{}]interface{})
		startIdx := 0
		if start, ok := template["Start"]; ok {
			startIdx = start.(int)
		}
		for i := startIdx; i < startIdx+template["Count"].(int); i++ {
			// fmt.Printf("HOSTNAME:peer%d\n", i)
			// fmt.Printf("CN:peer%d.%s\n", i, orgCrypto["Domain"].(string))
			cn := fmt.Sprintf("peer%d.%s", i, orgCrypto["Domain"].(string))
			menu.Option(cn, cn, false, nil)
		}
	}

	err = menu.Run()
	if err != nil {
		log.Fatal(err)
	}

	// pullCli("1.4.2")
	listCli(dfltPeer)

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
