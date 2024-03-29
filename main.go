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
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
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

func listCli(peer string) (string, string) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	var peerContainer types.Container
	var network string
	c := types.ContainerListOptions{}
	if list, err := cli.ContainerList(ctx, c); err != nil {
		panic(err)
	} else {
		for _, container := range list {
			if strings.Split(container.Names[0], "/")[1] == peer {
				fmt.Printf("Found : %s\n", peer)
				fmt.Print(container.HostConfig.NetworkMode)
				network = container.HostConfig.NetworkMode
				peerContainer = container
				break
			}
		}
	}

	cc := types.ExecConfig{AttachStdout: true, AttachStderr: true, Cmd: []string{"peer", "channel", "list"}, Env: []string{"FABRIC_LOGGING_SPEC=critical"}}
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

	var dfltChannel string
	actChannelFunc := func(opts []wmenu.Opt) error {
		for _, opt := range opts {
			dfltChannel = opt.Value.(string)
			fmt.Printf("%s is selected\n", dfltChannel)
		}
		return nil
	}

	menu := wmenu.NewMenu("Choose an default channel to connect network")
	menu.Action(actChannelFunc)

	content, _, _ := res.Reader.ReadLine() // Skip the first line
	for content, _, err = res.Reader.ReadLine(); err == nil; content, _, err = res.Reader.ReadLine() {
		ch := string(content)
		// fmt.Println(ch)
		menu.Option(ch, ch, false, nil)
	}

	err = menu.Run()
	if err != nil {
		log.Fatal(err)
	}

	return network, dfltChannel
}

func discoverPeers(peer string, net string, ch string, domain string, mspid string) {

	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	skname := "6b3cf4905408799eecf955fbb31325d24356c1cd042069a72887ab24d2455863_sk"
	cmd := fmt.Sprintf(`discover --configFile conf.yaml \
	--peerTLSCA=tls/ca.crt \
	--userKey=msp/keystore/%s \
	--userCert=msp/signcerts/User1@%s-cert.pem \
	--MSP %s \
	saveConfig; \
	discover --configFile conf.yaml \
	peers \
	--channel %s \
	--server %s:7051`, skname, domain, mspid, ch, peer)

	fmt.Println(cmd)
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image:        "hyperledger/fabric-tools:1.4.2",
		Cmd:          []string{"sh", "-c", cmd},
		Tty:          true,
		AttachStdout: true,
		AttachStderr: true,
		WorkingDir:   "/etc/hyperledger/fabric",
	}, &container.HostConfig{
		// AutoRemove: true,
		NetworkMode: container.NetworkMode(net),
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: "/home/atsushi/hyperledger/bc-explorer-setup/crypto-config/peerOrganizations/org1.example.com/users/User1@org1.example.com/msp",
				Target: "/etc/hyperledger/fabric/msp",
			},
			{
				Type:   mount.TypeBind,
				Source: "/home/atsushi/hyperledger/bc-explorer-setup/crypto-config/peerOrganizations/org1.example.com/users/User1@org1.example.com/tls",
				Target: "/etc/hyperledger/fabric/tls",
			},
		},
	}, nil, "")
	if err != nil {
		panic(err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		panic(err)
	}

	statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			panic(err)
		}
	case <-statusCh:
	}

	out, err := cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{ShowStderr: true, ShowStdout: true})
	if err != nil {
		panic(err)
	}

	data, _ := ioutil.ReadAll(out)
	// fmt.Println(string(data))
	discResp := []DiscoverResp{}
	json.Unmarshal(data, &discResp)
	// fmt.Println(discResp)

	for _, node := range discResp {
		fmt.Println(node.Endpoint)
	}
}

type DiscoverResp struct {
	MSPID        string
	LedgerHeight string
	Endpoint     string
	Identity     string
	Chaincodes   []string
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
	networkName, dfltChannel := listCli(dfltPeer)

	discoverPeers(dfltPeer, networkName, dfltChannel, domain, org["ID"].(string))

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
