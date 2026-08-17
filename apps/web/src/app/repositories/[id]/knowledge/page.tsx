import type {Metadata} from "next";
import {KnowledgeWorkspace} from "@/components/knowledge-workspace";
export const metadata:Metadata={title:"Project knowledge"};
export default async function KnowledgePage({params}:{params:Promise<{id:string}>}){const{id}=await params;return <KnowledgeWorkspace repositoryID={id}/>}
