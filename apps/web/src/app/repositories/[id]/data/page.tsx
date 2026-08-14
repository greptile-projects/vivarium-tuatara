import type { Metadata } from "next";
import { DataCommitmentsWorkspace } from "@/components/data-commitments-workspace";

export const metadata: Metadata = { title: "Data commitments and flows" };
export default async function DataPage({params}:{params:Promise<{id:string}>}) { const {id}=await params; return <DataCommitmentsWorkspace repositoryID={id}/>; }
