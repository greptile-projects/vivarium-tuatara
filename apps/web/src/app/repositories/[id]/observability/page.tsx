import type { Metadata } from "next";
import { ObservabilityGapsWorkspace } from "@/components/observability-gaps-workspace";
export const metadata:Metadata={title:"Observability gaps"};
export default async function ObservabilityPage({params}:{params:Promise<{id:string}>}){const{id}=await params;return <ObservabilityGapsWorkspace repositoryID={id}/>}
