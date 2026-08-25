package provenancegraphs

import (
	"errors"
	"testing"
)

func TestCreateReconcilesExactRequestAndRejectsChangedReuse(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil { t.Fatal(err) }
	g := Graph{RequestID:"request-1",RepositoryID:"repo",Revision:"0123456789abcdef0123456789abcdef01234567",CreatedBy:"person",Nodes:[]Node{{ID:"source",Kind:"upstream_project",Label:"upstream",Confidence:"verified",Audience:"public",Citations:[]Citation{}}}}
	first, err := s.Create(g)
	if err != nil { t.Fatal(err) }
	retry, err := s.Create(g)
	if err != nil || retry.ID != first.ID { t.Fatalf("retry = %#v, %v",retry,err) }
	g.Nodes[0].Label = "changed"
	if _, err := s.Create(g); !errors.Is(err,ErrRequestConflict) { t.Fatalf("changed reuse = %v",err) }
	reopened, _ := New(s.root)
	values, err := reopened.List("repo")
	if err != nil || len(values)!=1 || values[0].ID!=first.ID { t.Fatalf("persisted = %#v, %v",values,err) }
}

func TestGraphValidationRequiresClosedConnectedVocabulary(t *testing.T) {
	s,_:=New(t.TempDir())
	g:=Graph{RequestID:"r",RepositoryID:"repo",Revision:"0123456789abcdef0123456789abcdef01234567",CreatedBy:"person",Nodes:[]Node{{ID:"output",Kind:"file",Label:"output",Confidence:"verified",Audience:"public",Citations:[]Citation{}}},Edges:[]Edge{{ID:"edge",From:"missing",To:"output",Transformation:"invented",Confidence:"verified"}}}
	if _,err:=s.Create(g);!errors.Is(err,ErrInvalid){t.Fatalf("invalid graph = %v",err)}
}
