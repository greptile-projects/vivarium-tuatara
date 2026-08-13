import type { Metadata } from "next";
import { ProductOpportunitiesWorkspace } from "@/components/product-opportunities-workspace";
export const metadata: Metadata = { title: "Product opportunities" };
export default async function Page({params}:{params:Promise<{id:string}>}) { const {id}=await params; return <ProductOpportunitiesWorkspace repositoryID={id}/> }
