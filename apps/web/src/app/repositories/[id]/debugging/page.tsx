import type { Metadata } from "next";
import { DebuggingWorkspace } from "@/components/debugging-workspace";
export const metadata:Metadata={title:"Production debugging"};
export default async function Page({params}:{params:Promise<{id:string}>}){const {id}=await params;return <DebuggingWorkspace repositoryID={id}/>}
