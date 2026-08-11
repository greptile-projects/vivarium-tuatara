import type { Metadata } from "next";
import { IssueDetail } from "@/components/issues-workspace";

export const metadata: Metadata = { title: "Issue" };
export default async function IssuePage({params}:{params:Promise<{id:string;issueId:string}>}) { const {id,issueId}=await params; return <IssueDetail repositoryID={id} issueID={issueId}/>; }
