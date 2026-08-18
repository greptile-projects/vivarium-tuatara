package interfacechecks

import (
	"strings"
	"testing"
)

func TestEvidenceIsRevisionExactAndClassifiedOnce(t *testing.T) {
	s,err:=New(t.TempDir());if err!=nil{t.Fatal(err)}
	c,err:=s.Create(Check{RepositoryID:"repo",PullRequestID:"pull",Revision:strings.Repeat("a",40),PreviewID:"preview",DefinitionPath:".vivarium/interface-checks.json",DefinitionDigest:strings.Repeat("b",64),DesignProposalID:"design",DesignVersion:2,Name:"checkout",Journey:"complete checkout",Context:Context{Viewport:"mobile",Theme:"dark",Content:"long",Locale:"ar-EG",Interaction:"keyboard",AssistiveTechnology:"screen-reader"},Status:"failed",Coverage:[]string{"loading","error","success"},AffectedRequirements:[]string{"Keyboard checkout"},Differences:[]Difference{{ID:"diff-1",Kind:"behavioral",Summary:"focus skips payment",Requirement:"Keyboard checkout"}},Artifacts:[]Artifact{{ID:"recording",Kind:"recording",Name:"keyboard.webm",URL:"/artifact",Digest:strings.Repeat("c",64),SizeBytes:42}},Performance:[]Performance{{Metric:"interaction",Unit:"ms",Baseline:90,Candidate:110,Budget:120,Passed:true}}})
	if err!=nil{t.Fatal(err)}
	c,err=s.Classify("repo","pull",c.ID,"diff-1","regression","Focus order violates the accepted journey.","reviewer");if err!=nil{t.Fatal(err)}
	if len(c.Classifications)!=1||c.Classifications[0].ActorID!="reviewer"{t.Fatalf("unexpected classifications: %#v",c.Classifications)}
	if _,err=s.Classify("repo","pull",c.ID,"diff-1","false_positive","changed mind","other");err!=ErrConflict{t.Fatalf("expected immutable decision, got %v",err)}
}

func TestRejectsMissingContextAxis(t *testing.T) {
	s,_:=New(t.TempDir());_,err:=s.Create(Check{RepositoryID:"r",PullRequestID:"p",Revision:strings.Repeat("a",40)})
	if err!=ErrInvalid{t.Fatalf("expected invalid, got %v",err)}
}
