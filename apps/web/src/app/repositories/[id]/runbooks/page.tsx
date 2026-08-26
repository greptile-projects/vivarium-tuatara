import type { Metadata } from "next";
import { RunbooksWorkspace } from "@/components/runbooks-workspace";
export const metadata: Metadata = { title: "Operational runbooks" };
export default async function RunbooksPage({params}:{params:Promise<{id:string}>}){const {id}=await params;return <RunbooksWorkspace repositoryID={id}/>}
