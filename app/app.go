package app

import (
	"fmt"
	"github.com/JLevconoks/registryViewer/registry"
	"github.com/gdamore/tcell"
	"log"
	"os"
	"sort"
	"strings"
)

type NodeType int

const (
	TypeNode NodeType = iota
	TypeTag
)

type Node struct {
	nodeType NodeType
	children []*Node
	parent   *Node
	name     string
	expanded bool
	level    int
}

type App struct {
	registry        registry.Registry
	s               tcell.Screen
	width, height   int
	mainAreaHeight  int
	rootNode        *Node
	positions       []*Node
	cursorY         int
	scrollYOffset   int
	statusBarString string
}

func NewApp(registry registry.Registry) App {
	return App{registry: registry}
}

func (app *App) Run() {
	log.Println("Getting list from", app.registry.BaseUrl)
	repositories, err := app.registry.ListRepositories()
	if err != nil {
		log.Fatal("Error getting repositories: ", err)
	}

	if app.registry.SubPath != "" {
		prefix := strings.TrimPrefix(app.registry.SubPath, "/")
		filteredRepos := make([]string, 0)
		for _, r := range repositories {
			if strings.HasPrefix(r, prefix) {
				trimmed := strings.TrimPrefix(r, prefix)
				// This is to handle repository name where prefix is the repository name itself
				trimmed = strings.TrimPrefix(trimmed, "/")
				if trimmed != "" {
					filteredRepos = append(filteredRepos, trimmed)
				}
			}
		}
		repositories = filteredRepos
	}

	rootNode := Node{name: app.registry.BaseUrl + app.registry.SubPath, expanded: true, level: 0}
	rootNode.addAll(repositories)

	app.rootNode = &rootNode
	app.updatePositions()

	app.initScreen()
	// Catch panics and clean up because they mess up the terminal.
	defer func() {
		if p := recover(); p != nil {
			if app.s != nil {
				app.s.Fini()
			}
			panic(p)
		}
	}()

	app.refreshScreen()

	quit := make(chan struct{})
	go func() {
		for {
			ev := app.s.PollEvent()
			switch ev := ev.(type) {
			case *tcell.EventKey:
				switch ev.Key() {
				case tcell.KeyEscape, tcell.KeyCtrlC:
					close(quit)
					return
				case tcell.KeyEnter:
					app.KeyEnterHandler()
				case tcell.KeyDown:
					app.KeyDownHandler()
				case tcell.KeyUp:
					app.KeyUpHandler()
				case tcell.KeyRight:
					app.KeyRightHandler()
				case tcell.KeyLeft:
					app.KeyLeftHandler()
				}

				switch r := ev.Rune(); {
				case r >= 'a' && r <= 'z':
					app.RuneHandler(r)
				}
			}
		}
	}()
	<-quit

	app.s.Fini()
}

func (app *App) KeyEnterHandler() {
	if app.positions[app.cursorFullPosition()].expanded {
		app.KeyLeftHandler()
	} else {
		app.KeyRightHandler()
	}
}

func (app *App) KeyDownHandler() {
	app.moveCursor(1)
	node := app.positions[app.cursorFullPosition()]
	app.statusBarString = node.fullPath()
	app.refreshScreen()
}

func (app *App) KeyUpHandler() {
	app.moveCursor(-1)
	node := app.positions[app.cursorFullPosition()]
	app.statusBarString = node.fullPath()
	app.refreshScreen()
}

func (app *App) KeyRightHandler() {
	curFullPos := app.cursorFullPosition()

	node := app.positions[curFullPos]

	if node.nodeType == TypeNode && !node.expanded {
		app.positions[curFullPos].expanded = true

		// If node does not have children, then we need to get tags for this node.
		if len(app.positions[curFullPos].children) == 0 {

			// Build image path.
			imagePath := ""
			for n := app.positions[curFullPos]; n.level > 0; n = n.parent {
				imagePath = "/" + n.name + imagePath
			}

			tags, err := app.registry.Tags(imagePath)
			if err != nil {
				app.statusBarString = err.Error()
				return
			}

			// Convert tag list into Node structs and sort
			tagNodes := make([]*Node, len(tags))
			for i, t := range tags {
				node := &Node{nodeType: TypeTag, parent: node, name: t, level: node.level + 1}
				tagNodes[i] = node
			}

			sort.Slice(tagNodes, func(i, j int) bool {
				return tagNodes[i].name < tagNodes[j].name
			})

			node.children = tagNodes
		}
	}

	app.updatePositions()
	app.refreshScreen()
}

func (app *App) KeyLeftHandler() {
	curFullPos := app.cursorFullPosition()
	if app.positions[curFullPos].expanded {
		app.positions[curFullPos].expanded = false
	} else {
		// Move up until you reach parent
		for i := curFullPos - 1; i >= 0; i-- {
			if app.positions[i].level < app.positions[curFullPos].level {
				diff := curFullPos - i
				app.moveCursor(-diff)
				break
			}
		}
	}

	app.updatePositions()
	app.refreshScreen()
}

func (app *App) RuneHandler(r rune) {
	curFulPos := app.cursorFullPosition()

	// Search to bottom
	prefix := string(r)
	moveCount := 0
	for index, value := range app.positions[curFulPos+1:] {
		if strings.HasPrefix(value.name, prefix) {
			moveCount = index + 1
			break
		}
	}

	// If nowhere to move, try searching from top to current position
	if moveCount == 0 {
		for index, value := range app.positions[:curFulPos] {
			if strings.HasPrefix(value.name, prefix) {
				moveCount = -curFulPos + index
				break
			}
		}
	}

	app.moveCursor(moveCount)
	app.refreshScreen()
}

func (app *App) initScreen() {
	screen, e := tcell.NewScreen()
	if e != nil {
		fmt.Printf("%v\n", e)
		os.Exit(1)
	}

	err := screen.Init()
	if err != nil {
		log.Fatal(err)
	}

	sw, sh := screen.Size()
	app.s = screen
	app.width = sw
	app.height = sh
	app.mainAreaHeight = sh - 2
}

func (app *App) refreshScreen() {
	app.s.Clear()

	for i := 0; i < app.mainAreaHeight; i++ {
		index := i + app.scrollYOffset
		if index < len(app.positions) {
			node := app.positions[index]

			styleCheck := i == app.cursorY
			if node != nil {
				printString(app.s, node.name, 3*node.level, i, tcell.StyleDefault.Reverse(styleCheck))
			}
		} else {
			break
		}
	}

	printString(app.s, app.statusBarString, 0, app.height-1, tcell.StyleDefault)

	app.s.Show()
}

func (app *App) updatePositions() {
	newPositions := toPositions(app.rootNode)
	app.positions = newPositions
}

func (node *Node) addAll(ss []string) {
	for _, s := range ss {
		node.addNode(s)
	}
}

func (app *App) cursorFullPosition() int {
	return app.cursorY + app.scrollYOffset
}

// moveCursor will move cursor n positions from current position.
func (app *App) moveCursor(ny int) {
	if len(app.positions) == 0 {
		return
	}

	cursorFullPosition := app.cursorFullPosition()
	newCursorY := app.cursorY
	newOffset := app.scrollYOffset

	if ny > 0 {
		// Moving down.
		maxPosition := len(app.positions) - 1
		if cursorFullPosition == maxPosition {
			return
		}
		if cursorFullPosition+ny > maxPosition {
			// If we are outside positions, change to max available movement.
			ny = len(app.positions) - 1 - app.cursorY - app.scrollYOffset
		}

		// Now we need to move.
		if app.cursorY+ny <= app.mainAreaHeight-1 {
			// No need to scroll
			newCursorY += ny
		} else {
			// Go to the bottom line and scroll
			addToY := app.mainAreaHeight - 1 - app.cursorY
			newCursorY += addToY
			newOffset += ny - addToY
		}

	} else if ny < 0 {
		// Moving up.

		ny = -ny
		if cursorFullPosition <= ny {
			// If trying to move up higher than possible, just set cursor to top position.
			newCursorY = 0
			newOffset = 0
		} else if app.cursorY >= ny {
			// We can move up without scrolling.
			newCursorY = app.cursorY - ny
		} else {
			// We need to move to the top of the screen and scroll.
			subFromOffset := ny - app.cursorY
			newCursorY = 0
			newOffset = app.scrollYOffset - subFromOffset
		}
	}

	app.cursorY = newCursorY
	app.scrollYOffset = newOffset
}

func (node *Node) addNode(s string) {
	index := strings.Index(s, "/")
	if index > 0 {
		newNode := node.getChildOrNew(s[:index])
		newNode.addNode(s[index+1:])
	} else {
		node.getChildOrNew(s)
	}
}

func (node *Node) getChildOrNew(name string) *Node {
	for index := range node.children {
		if node.children[index].name == name {
			return node.children[index]
		}
	}
	newNode := Node{nodeType: TypeNode, name: name, parent: node, level: node.level + 1}
	if len(node.children) == 0 {
		node.children = []*Node{&newNode}
	} else {
		node.children = append(node.children, &newNode)
	}
	return &newNode
}

func (node *Node) fullPath() string {
	path := ""

	for n := node; n.level != 0; n = n.parent {
		separator := "/"
		if n.nodeType == TypeTag {
			separator = ":"
		}
		path = separator + n.name + path
		if n.level == 1 {
			path = n.parent.name + path
		}
	}

	return path
}

func toPositions(root *Node) []*Node {
	positions := make([]*Node, 1)
	positions[0] = root

	if root.expanded {
		for index, _ := range root.children {
			positions = append(positions, toPositions(root.children[index])...)
		}
	}

	return positions
}

func printString(s tcell.Screen, str string, x, y int, style tcell.Style) {
	clearLine(s, y)
	for i, v := range str {
		s.SetContent(x+i, y, v, nil, style)
	}
}

func clearLine(s tcell.Screen, y int) {
	w, _ := s.Size()
	for x := 0; x < w; x++ {
		s.SetContent(x, y, rune(' '), nil, tcell.StyleDefault)
	}
}
