import type { Metadata } from "next";
import { PropagationCampaignsWorkspace } from "@/components/propagation-campaigns-workspace";
export const metadata:Metadata={title:"Propagation campaigns"};
export default async function Page({params}:{params:Promise<{id:string}>}){const {id}=await params;return <PropagationCampaignsWorkspace repositoryID={id}/>}
