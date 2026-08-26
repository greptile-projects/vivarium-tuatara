import type { Metadata } from "next";
import { CapacityObjectivesWorkspace } from "@/components/capacity-objectives-workspace";
import { CapacityModelsWorkspace } from "@/components/capacity-models-workspace";
import { CapacityTestsWorkspace } from "@/components/capacity-tests-workspace";

export const metadata: Metadata = { title: "Capacity objectives" };
export default async function CapacityPage({params}:{params:Promise<{id:string}>}){const {id}=await params;return <div className="space-y-12"><CapacityObjectivesWorkspace repositoryID={id}/><CapacityModelsWorkspace repositoryID={id}/><CapacityTestsWorkspace repositoryID={id}/></div>}
