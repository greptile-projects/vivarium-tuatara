import type { Metadata } from "next";
import { AdoptionCampaignsWorkspace } from "@/components/adoption-campaigns-workspace";
export const metadata:Metadata={title:"Release adoption"};
export default async function Page({params}:{params:Promise<{id:string}>}){const{id}=await params;return <AdoptionCampaignsWorkspace repositoryID={id}/>}
