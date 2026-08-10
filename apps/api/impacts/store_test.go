package impacts

import (
	"errors"
	"testing"
)

func TestAssessmentRetainsRefinementAndOwnerDecision(t *testing.T) {
	store,err:=New(t.TempDir());if err!=nil{t.Fatal(err)}
	v,err:=store.Create(Assessment{RepositoryID:"repo",Revision:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",Title:"Change authorization",Source:Source{Kind:"selected_code",Path:"auth.go",StartLine:10,EndLine:12},CreatedBy:"author",Items:[]Item{{Kind:"reference",Summary:"authorize call",Status:"candidate",AddedBy:"agent"}}});if err!=nil{t.Fatal(err)}
	v,err=store.AddParticipant(v.ID,v.Version,"author","reviewer");if err!=nil{t.Fatal(err)}
	v,err=store.AddItem(v.ID,v.Version,Item{Kind:"risk",Summary:"legacy clients may fail",Status:"accepted_risk",AddedBy:"reviewer"});if err!=nil{t.Fatal(err)}
	v,err=store.Request(v.ID,v.Version,AcknowledgementRequest{RepositoryID:"consumer",OwnerID:"consumer-owner",RequestedBy:"author",Note:"confirm migration coverage"});if err!=nil{t.Fatal(err)}
	v,err=store.Acknowledge(v.ID,v.Version,v.AcknowledgementRequests[0].ID,"consumer-owner","migration test named");if err!=nil{t.Fatal(err)}
	reopened,err:=store.Get(v.ID);if err!=nil{t.Fatal(err)}
	if reopened.Version!=5||len(reopened.Participants)!=2||len(reopened.Items)!=2||reopened.Items[1].Status!="accepted_risk"||reopened.AcknowledgementRequests[0].AcknowledgedBy!="consumer-owner"{t.Fatalf("assessment evidence was not retained: %#v",reopened)}
	if _,err=store.AddItem(v.ID,3,Item{Kind:"unknown",Summary:"stale write",AddedBy:"author"});!errors.Is(err,ErrConflict){t.Fatalf("stale update = %v",err)}
}

func TestOnlyNamedOwnerCanAcknowledge(t *testing.T){store,_:=New(t.TempDir());v,_:=store.Create(Assessment{RepositoryID:"repo",Revision:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",Title:"Change",Source:Source{Kind:"proposed_diff",Diff:"+change"},CreatedBy:"author",Items:[]Item{{Kind:"unknown",Summary:"unknown",AddedBy:"agent"}}});v,_=store.Request(v.ID,v.Version,AcknowledgementRequest{RepositoryID:"consumer",OwnerID:"owner",RequestedBy:"author"});if _,err:=store.Acknowledge(v.ID,v.Version,v.AcknowledgementRequests[0].ID,"other","");!errors.Is(err,ErrNotFound){t.Fatalf("unaffected actor acknowledged: %v",err)}}
