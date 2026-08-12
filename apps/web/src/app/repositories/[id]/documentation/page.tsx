import type { Metadata } from "next";
import { DocumentationWorkspace } from "@/components/documentation-workspace";
export const metadata:Metadata={title:"Documentation"};
export default async function Page({params}:{params:Promise<{id:string}>}){const {id}=await params;return <DocumentationWorkspace repositoryId={id}/>}
