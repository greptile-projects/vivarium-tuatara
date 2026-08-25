import type { Metadata } from "next";
import { CapacityObjectivesWorkspace } from "@/components/capacity-objectives-workspace";

export const metadata: Metadata = { title: "Capacity objectives" };
export default async function CapacityPage({params}:{params:Promise<{id:string}>}){const {id}=await params;return <CapacityObjectivesWorkspace repositoryID={id}/>}
