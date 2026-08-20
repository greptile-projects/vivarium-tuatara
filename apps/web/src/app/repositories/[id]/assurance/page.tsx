import type { Metadata } from "next";
import { AssuranceProgramsWorkspace } from "@/components/assurance-programs-workspace";
export const metadata:Metadata={title:"Assurance program"};
export default async function AssurancePage({params}:{params:Promise<{id:string}>}){const {id}=await params;return <AssuranceProgramsWorkspace repositoryID={id}/>}
