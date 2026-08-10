import type { Metadata } from "next";
import { ImpactWorkspace } from "@/components/impact-workspace";

export const metadata:Metadata={title:"Prospective change impact"};
export default async function ImpactPage({params,searchParams}:{params:Promise<{id:string}>;searchParams:Promise<Record<string,string|string[]|undefined>>}){const {id}=await params;const query=await searchParams;const one=(key:string)=>typeof query[key]==="string"?query[key] as string:"";return <ImpactWorkspace repositoryID={id} initialRef={one("ref")} initialPath={one("path")} initialStartLine={Number(one("line"))||1}/>;}
