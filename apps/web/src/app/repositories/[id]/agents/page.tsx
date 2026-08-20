import type { Metadata } from "next";
import { AgentProjectsWorkspace } from "@/components/agent-projects-workspace";
export const metadata:Metadata={title:"Agent projects"};
export default async function AgentProjectsPage({params}:{params:Promise<{id:string}>}){const {id}=await params;return <AgentProjectsWorkspace repositoryID={id}/>}
