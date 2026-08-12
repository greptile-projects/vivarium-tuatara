import type {Metadata} from "next";
import {DocumentationTaskWorkspace} from "@/components/documentation-task-workspace";
export const metadata:Metadata={title:"Documentation task"};
export default async function Page({params}:{params:Promise<{id:string;taskId:string}>}){const {id,taskId}=await params;return <DocumentationTaskWorkspace repositoryId={id} taskId={taskId}/>}
