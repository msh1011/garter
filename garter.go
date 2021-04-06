package garter

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	_ "embed"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v2"
)

var swaggerHeader = `swagger: "2.0"
info:
  description: "%s"
  version: "%s"
  title: "%s"
`

//go:embed swaggerui.html
var swaggerPage string

type Server struct {
	cmdTree     *CmdNode
	rootName    string
	execPath    string
	description string
	version     string
}

type param struct {
	In       string            `yaml:"in"`
	Typename string            `yaml:"type"`
	Name     string            `yaml:"name"`
	Items    map[string]string `yaml:"items,omitempty"`
}
type response struct {
	Description string
}
type entry struct {
	Summary    string
	Parameters []param
	Responses  map[string]response
}

type CmdNode struct {
	name  string
	child map[string]*CmdNode
	flags []*pflag.Flag
}

// Create a new garter server object, implements http.Handler
func NewServer(rootCmd *cobra.Command) (*Server, error) {
	runner, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return &Server{
		cmdTree:     cmdTree(rootCmd, nil),
		rootName:    rootCmd.Use,
		execPath:    runner,
		description: rootCmd.Long,
		version:     rootCmd.Version,
	}, nil
}

// AddServerCommand adds a basic server subcommand which will host a local
// server on 'portFlag' with timeouts set to timeoutFlag
func AddServerCmd(rootCmd *cobra.Command,
	portFlag *int, timeoutFlag *time.Duration) {

	rootCmd.AddCommand(&cobra.Command{
		Use:    "server",
		Hidden: true,
		Long: `
HTTP Server for commands build with cobra,
use /swaggerui to see available commands`,
		Run: func(cmd *cobra.Command, args []string) {
			ser, err := NewServer(rootCmd)
			if err != nil {
				panic(err)
			}
			address := fmt.Sprintf(":%d", *portFlag)
			srv := &http.Server{
				Handler:      ser,
				Addr:         address,
				WriteTimeout: *timeoutFlag,
				ReadTimeout:  *timeoutFlag,
			}
			log.Printf("Starting Garter server for %s at %s", ser.execPath, address)
			log.Fatal(srv.ListenAndServe())
		},
	})
}

func runCommand(command string, args []string) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.Command(command, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", "", err
	}
	return stdout.String(), stderr.String(), nil

}

func cmdTree(c *cobra.Command, inherited []*pflag.Flag) *CmdNode {
	var inh []*pflag.Flag

	n := &CmdNode{
		child: map[string]*CmdNode{},
		name:  c.Use,
	}

	for _, v := range inherited {
		n.flags = append(n.flags, v)
	}

	c.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		inh = append(inh, f)
	})
	c.Flags().VisitAll(func(f *pflag.Flag) {
		n.flags = append(n.flags, f)
	})

	for _, v := range c.Commands() {
		// Ignore hidden, help & completion commands
		if v.Hidden || strings.HasPrefix(v.Use, "help") ||
			strings.HasPrefix(v.Use, "completion") {
			continue
		}
		n.child[v.Use] = cmdTree(v, inh)
	}
	return n
}

func (n *CmdNode) toPathEntry() entry {
	m := entry{
		Responses: map[string]response{
			"200": {"OK"},
		},
		Parameters: []param{},
	}
	for _, v := range n.flags {
		m.Parameters = append(m.Parameters,
			param{"query", "string", v.Name, nil})
	}
	m.Parameters = append(m.Parameters, param{"query", "array", "argv",
		map[string]string{"type": "string"}})
	return m
}

func (n *CmdNode) String() string {
	if len(n.flags) == 0 && len(n.child) == 0 {
		return n.name
	}

	var args []string
	for _, v := range n.flags {
		args = append(args, v.Name)
	}
	var subcmds []string
	for _, v := range n.child {
		subcmds = append(subcmds, v.String())
	}

	return fmt.Sprintf("%s: (%s) [%s]", n.name, strings.Join(args, ", "),
		strings.Join(subcmds, ", "))
}

func (s *Server) SetDescription(v string) {
	s.description = v
}
func (s *Server) getDescription() string {
	if s.description == "" {
		return ""
	}
	return s.description
}

func (s *Server) SetVersion(v string) {
	s.version = v
}
func (s *Server) getVersion() string {
	if s.version == "" {
		return "1.0.0"
	}
	return s.version
}

func (s *Server) serverSwaggerUI(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, swaggerPage)
}

func (s *Server) serverSwaggerFile(w http.ResponseWriter) {

	paths := map[string]map[string]entry{}

	var addNode func(n *CmdNode, path string)

	addNode = func(n *CmdNode, path string) {
		nPath := fmt.Sprintf("%s/%s", path, n.name)
		paths[nPath] = map[string]entry{
			"get": n.toPathEntry(),
		}
		for _, v := range n.child {
			addNode(v, nPath)
		}
	}

	addNode(s.cmdTree, "")

	d, err := yaml.Marshal(map[string]interface{}{
		"paths": paths,
	})

	if err != nil {
		err1 := fmt.Errorf("Error: Failed to generate swagger file: %v", err.Error())
		log.Println(err1)
		http.Error(w, err1.Error(), 500)
		return
	}

	fmt.Fprintf(w, "%s%s", fmt.Sprintf(swaggerHeader,
		s.getDescription(), s.getVersion(), s.rootName), string(d))
}

func (s *Server) generateCommand(url *url.URL) (string, []string, error) {
	node := s.cmdTree
	args := []string{}

	// remove prefix from url path
	for _, v := range strings.Split(url.Path, "/")[2:] {
		if n, ok := node.child[v]; !ok {
			return "", nil, fmt.Errorf("Exepected path in url.")
		} else {
			node = n
			args = append(args, v)
		}
	}

	// Add cmd arguments from query
	q := url.Query()
	for _, v := range node.flags {
		if val := q.Get(v.Name); val != "" {
			args = append(args, fmt.Sprintf("--%s=%s", v.Name, val))
		}
	}

	if val := q.Get("argv"); val != "" {
		args = append(args, strings.Split(val, ",")...)
	}

	return s.execPath, args, nil

}

func (s *Server) serveRun(w http.ResponseWriter, r *http.Request) {

	if !strings.HasPrefix(r.URL.Path, "/"+s.rootName) {
		return
	}

	command, args, err := s.generateCommand(r.URL)
	if err != nil {
		err1 := fmt.Errorf("Error: Failed to generate command: %v", err.Error())
		log.Println(err1)
		http.Error(w, err1.Error(), 500)
		return
	}

	stdout, stderr, err := runCommand(command, args)
	if err != nil {
		err1 := fmt.Errorf("Error: Failed to run command: %v", err.Error())
		log.Println(err1)
		http.Error(w, err1.Error(), 500)
		return
	}

	resp := stdout
	if stderr != "" {
		resp += fmt.Sprintf("(%s)", stderr)
	}

	fmt.Fprint(w, resp)

}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Return the swagger-ui webpage
	log.Printf("Request from %s: %v", r.RemoteAddr, r.RequestURI)

	switch r.URL.Path {
	case "/swaggerui":
		s.serverSwaggerUI(w)
	case "/swagger":
		s.serverSwaggerFile(w)
	default:
		s.serveRun(w, r)
	}
}
