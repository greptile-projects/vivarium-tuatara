import type {Metadata} from "next";
import {CollaborationWorkflowsWorkspace} from "@/components/collaboration-workflows-workspace";
export const metadata:Metadata={title:"Collaboration workflows"};
export default async function WorkflowsPage({params}:{params:Promise<{id:string}>}){const {id}=await params;return <CollaborationWorkflowsWorkspace repositoryID={id}/>}
