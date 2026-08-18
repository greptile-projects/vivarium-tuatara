import type { Metadata } from "next";
import { InfrastructureWorkspace } from "@/components/infrastructure-workspace";

export const metadata: Metadata = { title: "Infrastructure" };
export default async function InfrastructurePage({params}:{params:Promise<{id:string}>}) { const {id}=await params; return <InfrastructureWorkspace repositoryID={id}/>; }
