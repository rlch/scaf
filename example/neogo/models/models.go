// Package models contains example Neo4j entity definitions using neogo.
// These models demonstrate the full range of neogo features including
// nodes, relationships, and various relationship patterns.
package models

import "github.com/rlch/neogo"

// Person represents a person node in the graph.
// Note: Self-referential relationships (Person -> Person) require a relationship
// struct to avoid Go's recursive type limitations.
type Person struct {
	neogo.Node `neo4j:"Person"`

	Name string `neo4j:"name"`
	Born int    `neo4j:"born"`

	// Relationships using relationship structs
	ActedIn  neogo.Many[ActedIn]  `neo4j:"->"`
	Directed neogo.Many[Directed] `neo4j:"->"`
	Reviewed neogo.Many[Review]   `neo4j:"->"`
	Follows  neogo.Many[Follows]  `neo4j:"->"`
}

// Movie represents a movie node in the graph.
type Movie struct {
	neogo.Node `neo4j:"Movie"`

	Title    string `neo4j:"title"`
	Released int    `neo4j:"released"`
	Tagline  string `neo4j:"tagline"`
}

// ActedIn represents the ACTED_IN relationship between a Person and a Movie.
type ActedIn struct {
	neogo.Relationship `neo4j:"ACTED_IN"`

	Roles []string `neo4j:"roles"`

	// Optional: explicit start/end node references
	Person *Person `neo4j:"startNode"`
	Movie  *Movie  `neo4j:"endNode"`
}

// Directed represents the DIRECTED relationship between a Person and a Movie.
type Directed struct {
	neogo.Relationship `neo4j:"DIRECTED"`

	Person *Person `neo4j:"startNode"`
	Movie  *Movie  `neo4j:"endNode"`
}

// Review represents the REVIEWED relationship between a Person and a Movie.
type Review struct {
	neogo.Relationship `neo4j:"REVIEWED"`

	Summary string `neo4j:"summary"`
	Rating  int    `neo4j:"rating"`

	Person *Person `neo4j:"startNode"`
	Movie  *Movie  `neo4j:"endNode"`
}

// Follows represents the FOLLOWS relationship between two Persons.
type Follows struct {
	neogo.Relationship `neo4j:"FOLLOWS"`

	Since int `neo4j:"since"`

	Follower *Person `neo4j:"startNode"`
	Followed *Person `neo4j:"endNode"`
}
