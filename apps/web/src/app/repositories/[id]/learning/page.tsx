import type { Metadata } from "next";
import { LearningPathwaysWorkspace } from "@/components/learning-pathways-workspace";
export const metadata: Metadata = { title: "Project learning" };
export default async function LearningPage({params}:{params:Promise<{id:string}>}) { const {id}=await params;return <LearningPathwaysWorkspace repositoryId={id}/> }
