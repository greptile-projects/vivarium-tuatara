import type {Metadata} from "next";
import {ProductExperimentsWorkspace} from "@/components/product-experiments-workspace";
export const metadata:Metadata={title:"Product experiments"};
export default async function Page({params}:{params:Promise<{id:string}>}){const {id}=await params;return <ProductExperimentsWorkspace repositoryID={id}/>}
