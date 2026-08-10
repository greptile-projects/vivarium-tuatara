import type { Metadata } from "next";
import { ExplanationWorkspace } from "@/components/explanation-workspace";

export const metadata:Metadata={title:"Ask the codebase"};
export default async function ExplanationPage({params,searchParams}:{params:Promise<{id:string}>;searchParams:Promise<Record<string,string|string[]|undefined>>}){const {id}=await params;const query=await searchParams;const one=(key:string)=>typeof query[key]==="string"?query[key] as string:"";return <ExplanationWorkspace repositoryID={id} initialRef={one("ref")} initialPath={one("path")} initialKind={(one("kind")||"repository") as "repository"|"file"|"proposal"|"task"|"pull_request"|"incident"|"workspace"} initialResourceID={one("resource_id")}/>;}
