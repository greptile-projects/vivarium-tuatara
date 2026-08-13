import type { Metadata } from "next";
import { PerformanceGoalsWorkspace } from "@/components/performance-goals-workspace";
export const metadata:Metadata={title:"Performance goals"};
export default async function PerformancePage({params}:{params:Promise<{id:string}>}){const {id}=await params;return <PerformanceGoalsWorkspace repositoryID={id}/>}
