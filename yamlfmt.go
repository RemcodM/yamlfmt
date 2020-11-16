package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type queueItem struct {
	Node *yaml.Node
	Path []string
	Indent int
}

type tupleItem struct {
	Key *yaml.Node
	Value *yaml.Node
}

func main() {
	overwrite := flag.Bool("w", false, "overwrite the input file")
	indent := flag.Int("indent", 2, "default indent")
	debug := flag.Bool("d", false, "show debug output on stderr")
	flag.Parse()

	if flag.NArg() > 0 {
		for _, f := range flag.Args() {
			formatFile(f, *indent, *overwrite, *debug)
		}
	} else {
		formatStream(os.Stdin, os.Stdout, *indent, *debug)
	}
}

func formatFile(f string, indent int, overwrite bool, debug bool) {
	r, err := os.Open(f)
	if err != nil {
		log.Fatal(err)
	}

	var out bytes.Buffer
	if e := formatStream(r, &out, indent, debug); e != nil {
		log.Fatalf("Failed formatting YAML stream: %v", e)
	}

	r.Close()

	if e := dumpStream(&out, f, overwrite); e != nil {
		log.Fatalf("Cannot overwrite: %v", e)
	}
}

func formatStream(r io.Reader, out io.Writer, indent int, debug bool) error {
	d := yaml.NewDecoder(r)
	in := &yaml.Node{}
	err := d.Decode(in)
	docs := []*yaml.Node{}

	for err == nil {
		docs = append(docs, in)
		in = &yaml.Node{}
		err = d.Decode(in)
	}

	if err != nil && err != io.EOF {
		return err
	}

	sort.Slice(docs, func(i, j int) bool {
		return sortDocument(docs[i], docs[j])
	});

	/* node, err2 := traverse(&in, "metadata", "name");
	if err2 != nil {
		fmt.Println(err2)
	} else {
		fmt.Println(node.Value)
	} */

	e := yaml.NewEncoder(out)
	e.SetIndent(indent)

	for _, doc := range docs {
		normalize(doc, debug)
		if err := e.Encode(doc); err != nil {
			log.Fatal(err)
		}
	}

	e.Close()

	return nil
}

func sortDocument(i *yaml.Node, j *yaml.Node) bool {
	kind_i, err_kind_i := traverse(i, "kind")
	kind_j, err_kind_j := traverse(j, "kind")
	if err_kind_i != nil && err_kind_j == nil {
		return false
	} else if err_kind_j != nil && err_kind_i == nil {
		return true
	} else if err_kind_i == nil && err_kind_j == nil && kind_i.Value != kind_j.Value {
		return kind_i.Value < kind_j.Value
	}

	ns_i, err_ns_i := traverse(i, "metadata", "namespace")
	ns_j, err_ns_j := traverse(j, "metadata", "namespace")
	if err_ns_i != nil && err_ns_j == nil {
		return false
	} else if err_ns_j != nil && err_ns_i == nil {
		return true
	} else if err_ns_i == nil && err_ns_j == nil && ns_i.Value != ns_j.Value {
		return ns_i.Value < ns_j.Value
	}

	name_i, err_i := traverse(i, "metadata", "name")
	name_j, err_j := traverse(j, "metadata", "name")
	if err_i != nil && err_j == nil {
		return false
	} else if err_j != nil && err_i == nil {
		return true
	} else if err_i == nil && err_j == nil && name_i.Value != name_j.Value {
		return name_i.Value < name_j.Value
	}

	return false;
}

func normalize(node *yaml.Node, debug bool) {
	stack := []queueItem{
		queueItem { Node: node, Path: []string{}, Indent: 0 },
	}
	var top queueItem
	for len(stack) > 0 {
		top, stack = stack[0], stack[1:]
		if debug {
			printNode(top.Node, top.Path, top.Indent)
		}
		normalizeStyle(&top)

		content := []queueItem{}

		if top.Node.Kind & yaml.SequenceNode > 0 {
			for index, child := range top.Node.Content {
				path := append([]string{}, top.Path...)
				content = append(content, queueItem { Node: child, Path: append(path, strconv.Itoa(index)), Indent: top.Indent + 1 })
			}
		} else if top.Node.Kind & yaml.MappingNode > 0 {
			tuples, _ := tuples(top.Node.Content)
			for _, tuple := range tuples {
				content = append(content, queueItem { Node: tuple.Key, Path: top.Path, Indent: top.Indent + 1 })
				path := append([]string{}, top.Path...)
				content = append(content, queueItem { Node: tuple.Value, Path: append(path, tuple.Key.Value), Indent: top.Indent + 1 })
			}
			sort.Slice(tuples, func(i, j int) bool {
				return tuples[i].Key.Value < tuples[j].Key.Value
			});
			top.Node.Content = contents(tuples)
		} else {
			for _, child := range top.Node.Content {
				content = append(content, queueItem { Node: child, Path: top.Path, Indent: top.Indent + 1 })
			}
		}

		stack = append(content, stack...)
	}
}

func normalizeStyle(item *queueItem) {
	if item.Node.Style & yaml.SingleQuotedStyle > 0 {
		item.Node.Style = item.Node.Style ^ yaml.SingleQuotedStyle
	}
	if item.Node.Style & yaml.DoubleQuotedStyle > 0 {
		item.Node.Style = item.Node.Style ^ yaml.DoubleQuotedStyle
	}
	if item.Node.Style & yaml.FlowStyle > 0 {
		item.Node.Style = item.Node.Style ^ yaml.FlowStyle
	}
}

func mapping(s []*yaml.Node) (map[string]*yaml.Node, error) {
	i := 0
	r := make(map[string]*yaml.Node)
	if len(s) % 2 != 0 {
		return r, errors.New("Mapping expected even number of nodes")
	}
	for i < len(s) {
		r[s[i].Value] = s[i + 1]
		i += 2
	}
	return r, nil
}

func tuples(s []*yaml.Node) ([]tupleItem, error) {
	i := 0
	r := []tupleItem{}
	if len(s) % 2 != 0 {
		return r, errors.New("Tuples expected even number of nodes")
	}
	for i < len(s) {
		r = append(r, tupleItem{ Key: s[i], Value: s[i + 1] })
		i += 2
	}
	return r, nil
}

func contents(s []tupleItem) []*yaml.Node {
	r := []*yaml.Node{}
	for _, i := range s {
		r = append(r, i.Key)
		r = append(r, i.Value)
	}
	return r
}

func traverse(node *yaml.Node, keys ...string) (*yaml.Node, error) {
	i := 0
	for i < len(keys) {
		if node.Kind & yaml.DocumentNode > 0 {
			if len(node.Content) != 1 {
				return nil, errors.New("Expected one child for DocumentNode")
			}
			node = node.Content[0]
		} else if node.Kind & yaml.SequenceNode > 0 {
			index, err := strconv.Atoi(keys[i])
			if err == nil {
				return nil, errors.New("Traversed to sequence node but got no index")
			}
			if index >= len(node.Content) {
				return nil, errors.New("Traversed to sequence node but index out of range")
			}
			node = node.Content[index]
			i++
		} else if node.Kind & yaml.MappingNode > 0 {
			mapping, err := mapping(node.Content)
			if err != nil {
				return nil, err
			}
			if value, ok := mapping[keys[i]]; ok {
				node = value
			} else {
				return nil, errors.New("Traversed to mapping node but key not in mapping")
			}
			i++
		} else if node.Kind & yaml.ScalarNode > 0 {
			return nil, errors.New("Traversed to ScalarNode, but not finished yet")
		} else if node.Kind & yaml.AliasNode > 0 {
			node = node.Alias
		}
	}
	return node, nil
}

func printNode(node *yaml.Node, path []string, indent int) {
	i := 0
	for i < indent {
		fmt.Fprint(os.Stderr, "  ")
		i++
	}
	fmt.Fprint(os.Stderr, "Node .")
	fmt.Fprint(os.Stderr, strings.Join(path, "."))
	fmt.Fprint(os.Stderr, ": ")
	fmt.Fprint(os.Stderr, node.Tag)
	fmt.Fprint(os.Stderr, " ")
	fmt.Fprint(os.Stderr, node.Value)
	fmt.Fprint(os.Stderr, " ")
	if node.Kind & yaml.DocumentNode > 0 {
		fmt.Fprint(os.Stderr, "DocumentNode ")
	}
	if node.Kind & yaml.SequenceNode > 0 {
		fmt.Fprint(os.Stderr, "SequenceNode ")
	}
	if node.Kind & yaml.MappingNode > 0 {
		fmt.Fprint(os.Stderr, "MappingNode ")
	}
	if node.Kind & yaml.ScalarNode > 0 {
		fmt.Fprint(os.Stderr, "ScalarNode ")
	}
	if node.Kind & yaml.AliasNode > 0 {
		fmt.Fprint(os.Stderr, "AliasNode ")
	}
	if node.Style & yaml.TaggedStyle > 0 {
		fmt.Fprint(os.Stderr, "TaggedStyle ")
	}
	if node.Style & yaml.DoubleQuotedStyle > 0 {
		fmt.Fprint(os.Stderr, "DoubleQuotedStyle ")
	}
	if node.Style & yaml.SingleQuotedStyle > 0 {
		fmt.Fprint(os.Stderr, "SingleQuotedStyle ")
	}
	if node.Style & yaml.LiteralStyle > 0 {
		fmt.Fprint(os.Stderr, "LiteralStyle ")
	}
	if node.Style & yaml.FoldedStyle > 0 {
		fmt.Fprint(os.Stderr, "FoldedStyle ")
	}
	if node.Style & yaml.FlowStyle > 0 {
		fmt.Fprint(os.Stderr, "FlowStyle ")
	}
	fmt.Fprintln(os.Stderr, "")
}

func dumpStream(out *bytes.Buffer, f string, overwrite bool) error {
	if overwrite {
		return ioutil.WriteFile(f, out.Bytes(), 0744)
	}
	_, err := io.Copy(os.Stdout, out)
	return err
}
