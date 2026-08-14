import type { Metadata } from "next";
import { ProjectFundsWorkspace } from "@/components/project-funds-workspace";
export const metadata: Metadata = { title: "Project funds" };
export default async function FundsPage({params}:{params:Promise<{id:string}>}) { const {id}=await params; return <ProjectFundsWorkspace repositoryID={id}/>; }
