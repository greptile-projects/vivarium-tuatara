import type { Metadata } from "next";
import { RoadmapWorkspace } from "@/components/roadmap-workspace";
export const metadata: Metadata = { title: "Roadmap" };
export default async function Page({params}:{params:Promise<{id:string}>}) { const {id}=await params; return <RoadmapWorkspace repositoryID={id}/> }
