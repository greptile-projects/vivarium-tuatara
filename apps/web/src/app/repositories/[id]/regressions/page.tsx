import type { Metadata } from "next";
import { RegressionInvestigationsWorkspace } from "@/components/regression-investigations-workspace";
export const metadata:Metadata={title:"Regression investigations"};
export default async function Page({params}:{params:Promise<{id:string}>}){const {id}=await params;return <RegressionInvestigationsWorkspace repositoryID={id}/>}
